// 实验 08（运行时调度器 GMP），断言验证
// 对应笔记：notes/golang/07-运行时调度器GMP.md
// 源码对照：src/runtime/runtime2.go（g 结构）、src/runtime/proc.go（sysmon → preemptM → SIGURG）
// 调度顺序本身不确定，这里只钉住可确定的行为边界
package main

import (
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGOMAXPROCSReturnsOldValue 第 1 节：P 数 = GOMAXPROCS，设置时返回旧值
func TestGOMAXPROCSReturnsOldValue(t *testing.T) {
	prev := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(prev)

	assert.Equal(t, 2, runtime.GOMAXPROCS(0), "P 的数量 = GOMAXPROCS")
	assert.GreaterOrEqual(t, prev, 1, "设置时返回旧值")
	assert.GreaterOrEqual(t, runtime.NumCPU(), 1)
}

// TestGoroutineGrowthAndFallback 第 2 节：千级 goroutine 秒建秒回收
func TestGoroutineGrowthAndFallback(t *testing.T) {
	base := runtime.NumGoroutine()

	done := make(chan struct{})
	for i := 0; i < 1000; i++ {
		go func() { <-done }() // 全部 park 在 channel 上
	}

	waitUntil(t, 2*time.Second, func() bool { return runtime.NumGoroutine() >= base+900 })
	assert.GreaterOrEqual(t, runtime.NumGoroutine(), base+900, "千级 goroutine 秒建（2KB 栈 + 用户态调度）")

	close(done)
	waitUntil(t, 2*time.Second, func() bool { return runtime.NumGoroutine() <= base+100 })
	assert.LessOrEqual(t, runtime.NumGoroutine(), base+100, "广播退出后回落到基线附近")
}

// TestGoschedBothGoroutinesFinish 第 3 节：单 P 下主动让出，双方都不饿死
func TestGoschedBothGoroutinesFinish(t *testing.T) {
	prev := runtime.GOMAXPROCS(1) // 单 P 下让出效果最直观
	defer runtime.GOMAXPROCS(prev)

	var a, b atomic.Int64
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 6; i++ {
			runtime.Gosched()
			a.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 6; i++ {
			runtime.Gosched()
			b.Add(1)
		}
	}()
	wg.Wait()

	assert.Equal(t, int64(6), a.Load())
	assert.Equal(t, int64(6), b.Load(), "Gosched 让出后另一个 G 能推进，双方都不饿死")
}

// TestAsyncPreemptKeepsTicksAlive 第 4 节：纯 CPU 循环被 SIGURG 异步抢占
func TestAsyncPreemptKeepsTicksAlive(t *testing.T) {
	prev := runtime.GOMAXPROCS(1) // 单 P：抢占一旦失效，主 G 会被饿死
	defer runtime.GOMAXPROCS(prev)

	done := make(chan struct{})
	go func() {
		var sum int64
		for i := int64(0); i < 1<<30; i++ { // 循环体内零函数调用 = 协作式抢占的盲区
			sum += i
		}
		_ = sum
		close(done)
	}()

	t0 := time.Now()
	for i := 1; i <= 3; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	elapsed := time.Since(t0)

	assert.Less(t, elapsed, 2*time.Second, "纯 CPU G 运行期间主 G 按时醒来：异步抢占（SIGURG）生效")
	assert.GreaterOrEqual(t, elapsed, 150*time.Millisecond)

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("纯 CPU 循环未在预期时间内结束")
	}
}

// TestBlockingCompareThreads 第 5 节：channel 阻塞不占线程，系统调用阻塞占住 M
func TestBlockingCompareThreads(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过（含 1s 系统调用阻塞）")
	}

	threads := func() int { return pprof.Lookup("threadcreate").Count() }

	// A) channel 阻塞：G 被 park，M/P 不受影响
	base := threads()
	parked := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-parked }()
	}
	time.Sleep(100 * time.Millisecond)
	parkedDelta := threads() - base
	close(parked)
	wg.Wait()

	assert.LessOrEqual(t, parkedDelta, 5, "100 个 G 阻塞在 channel：线程数几乎不变（park 的是 G）")

	// B) 系统调用阻塞：每个阻塞的 G 陪绑一个 M。
	// 并发数取「当前线程数 + 32」：超过现有线程（含 idle M）的容量，必然要新建 M，
	// 这样观测就不受 idle 池还剩多少影响（同进程重复跑也不会失效）。
	tv := syscall.Timeval{Sec: 1}
	base = threads()
	n := base + 32
	var wg2 sync.WaitGroup
	for i := 0; i < n; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			syscall.Select(0, nil, nil, nil, &tv) // 直接陷内核的阻塞系统调用
		}()
	}
	time.Sleep(300 * time.Millisecond)
	blockedThreads := threads()
	wg2.Wait()

	assert.GreaterOrEqual(t, blockedThreads, base+32,
		"阻塞的系统调用数超过现有线程容量：必须新建 M 兜住（M 被陪绑，P 被 hand off）")
}

// waitUntil 带上限地轮询等待条件成立（避免测试挂死）
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}
