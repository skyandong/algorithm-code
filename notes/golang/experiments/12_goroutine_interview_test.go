// 实验 12（Goroutine 面试题集），断言验证
// 对应笔记：notes/golang/11-Goroutine面试题集.md
// 源码对照：src/runtime/runtime2.go
package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestBlockingMapExisting 已存在的 key 立即返回。
func TestBlockingMapExisting(t *testing.T) {
	m := newBlockingMap()
	m.Out("k1", "v1")

	assert.Equal(t, "v1", m.Rd("k1", time.Second))
}

// TestBlockingMapWakeup 等待中的 Rd 被 Out 唤醒并拿到值。
func TestBlockingMapWakeup(t *testing.T) {
	m := newBlockingMap()

	type result struct {
		v  any
		at time.Time
	}
	done := make(chan result, 1)
	go func() {
		v := m.Rd("k2", 5*time.Second)
		done <- result{v: v, at: time.Now()}
	}()

	time.Sleep(50 * time.Millisecond)
	putAt := time.Now()
	m.Out("k2", "v2")

	got := <-done
	assert.Equal(t, "v2", got.v)
	assert.True(t, got.at.After(putAt), "Rd 应阻塞到 Out 之后才返回")
}

// TestBlockingMapTimeout 超时返回 nil，且不阻塞后续 Out。
func TestBlockingMapTimeout(t *testing.T) {
	m := newBlockingMap()

	assert.Nil(t, m.Rd("missing", 100*time.Millisecond))

	m.Out("missing", "late") // 超时的等待者应已被清理，不能 panic
	assert.Equal(t, "late", m.Rd("missing", time.Second))
}

// TestBanVisit 三分钟窗口内同 IP 只放行一次（注入时间，完全确定性）。
func TestBanVisit(t *testing.T) {
	ban := NewBan()
	t0 := time.Now()

	assert.False(t, ban.visit("1.2.3.4", t0), "首次访问放行")
	assert.True(t, ban.visit("1.2.3.4", t0.Add(time.Minute)), "窗口内再访被限")
	assert.True(t, ban.visit("1.2.3.4", t0.Add(2*time.Minute)), "窗口内再访被限")
	assert.False(t, ban.visit("1.2.3.4", t0.Add(3*time.Minute)), "窗口过期放行")
	assert.True(t, ban.visit("1.2.3.4", t0.Add(3*time.Minute+time.Second)), "重新计时")

	assert.False(t, ban.visit("5.6.7.8", t0), "不同 IP 互不影响")
}

// TestBanVisitConcurrency 100 个不同 IP 并发各访一次，恰好放行 100 次（无竞态）。
func TestBanVisitConcurrency(t *testing.T) {
	ban := NewBan()
	var success atomic.Int64
	var wg sync.WaitGroup
	wg.Add(100)
	for j := 0; j < 100; j++ {
		go func(ip string) {
			defer wg.Done()
			if !ban.visit(ip, time.Now()) {
				success.Add(1)
			}
		}(string(rune('A' + j)))
	}
	wg.Wait()
	assert.Equal(t, int64(100), success.Load(), "每个 IP 三分钟窗口内只允许一次")
}

// TestWaitTimeout 超时返回 true（调用方负责取消）、自然结束返回 false。
func TestWaitTimeout(t *testing.T) {
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-stop:
			case <-time.After(10 * time.Second):
			}
		}()
	}
	assert.True(t, WaitTimeout(&wg, 100*time.Millisecond))
	close(stop)
	wg.Wait()

	var wg2 sync.WaitGroup
	wg2.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg2.Done()
			time.Sleep(20 * time.Millisecond)
		}()
	}
	assert.False(t, WaitTimeout(&wg2, 2*time.Second))
}

// TestFindWithTimeoutFound 目标存在 → 找到即取消其余 worker。
func TestFindWithTimeoutFound(t *testing.T) {
	big := make([]int, 1_000_000)
	for i := range big {
		big[i] = i
	}
	assert.Equal(t, "found", findWithTimeout(big, 777777, 8, 5*time.Second))
}

// TestFindWithTimeoutNotFound 目标不存在且扫描很快完成 → notfound（未超时）。
func TestFindWithTimeoutNotFound(t *testing.T) {
	big := make([]int, 1_000_000)
	for i := range big {
		big[i] = i
	}
	assert.Equal(t, "notfound", findWithTimeout(big, -1, 8, 5*time.Second))
}

// TestFindSlowWithTimeout 目标不存在 + 慢查询 → 超时取消 → timeout。
func TestFindSlowWithTimeout(t *testing.T) {
	big := make([]int, 1_000_000)
	for i := range big {
		big[i] = i
	}
	assert.Equal(t, "timeout", findSlowWithTimeout(big, -1, 2, 300*time.Millisecond))
}

// TestRangeClosureValueCopy range 值副本自增不影响原 slice（题19 核心结论）。
func TestRangeClosureValueCopy(t *testing.T) {
	type rngT struct{ V int }
	incr := func(t *rngT, wg *sync.WaitGroup) {
		defer wg.Done()
		t.V++
	}

	var wg sync.WaitGroup
	wg.Add(10)
	ts := make([]rngT, 10)
	for i := 0; i < 10; i++ {
		ts[i] = rngT{i}
	}
	for _, tr := range ts {
		go incr(&tr, &wg) // 值副本自增，不影响 ts
	}
	wg.Wait()

	for i := range ts {
		assert.Equal(t, i, ts[i].V, "range 值副本自增不改原 slice 元素")
	}
}

// TestProducerConsumerChannel 发送方负责 close，接收方 range 收全后退出。
func TestProducerConsumerChannel(t *testing.T) {
	out := make(chan int)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer close(out) // 发送方负责关闭
		for i := 0; i < 5; i++ {
			out <- i
		}
	}()

	got := make([]int, 0, 5)
	go func() {
		defer wg.Done()
		for v := range out {
			got = append(got, v)
		}
	}()
	wg.Wait()

	assert.Equal(t, []int{0, 1, 2, 3, 4}, got, "发送方 close 后接收方 range 收全并退出")
}

// TestTimerPanicRecovered 同 goroutine 内 recover 能拦住 panic，定时调用不退出。
func TestTimerPanicRecovered(t *testing.T) {
	proc := func() { panic("boom") }
	callSafely := func(f func()) (recovered any) {
		defer func() { recovered = recover() }()
		f()
		return
	}
	for i := 0; i < 3; i++ {
		assert.Equal(t, "boom", callSafely(proc), "panic 被同 goroutine 的 recover 拦住")
	}
}
