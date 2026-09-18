// 实验 06（sync 锁与原子操作），断言验证
// 对应笔记：notes/golang/06-sync锁与原子操作.md
// 源码对照：src/internal/sync/mutex.go（sync.Mutex 本体在 internal/sync，src/sync/mutex.go 是薄包装）
// 子进程 fatal 验证：go test -run TestUnlockUnlockedMutexIsFatal ./experiments/
package main

import (
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// runWorkers 起 n 个 goroutine 跑 fn 并等全部结束
func runWorkers(n int, fn func()) {
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	wg.Wait()
}

// TestMutexForeignUnlock 第 1 节：Mutex 不记录属主，他人解锁也合法
func TestMutexForeignUnlock(t *testing.T) {
	var mu sync.Mutex
	mu.Lock()

	done := make(chan struct{})
	go func() {
		mu.Unlock() // Mutex 不记录属主，他人解锁合法
		close(done)
	}()
	<-done

	assert.True(t, mu.TryLock(), "他人 Unlock 后锁已释放（Mutex 无属主概念）")
	mu.Unlock()
}

// TestUnlockUnlockedMutexIsFatal 第 1 节：未加锁就 Unlock 是 fatal（子进程验证）
func TestUnlockUnlockedMutexIsFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过子进程崩溃验证")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestUnlockUnlockedMutexFatalChild", "-test.count=1")
	cmd.Env = append(os.Environ(), "SYNC_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()

	assert.Error(t, err, "fatal error 令子进程非零退出")
	assert.Contains(t, string(out), "sync: unlock of unlocked mutex", "子进程 fatal 输出含该错误文案")
	assert.NotContains(t, string(out), "panic:", "throw 不是 panic")
}

// TestUnlockUnlockedMutexFatalChild 子进程实体：触发 unlock of unlocked mutex
func TestUnlockUnlockedMutexFatalChild(t *testing.T) {
	if os.Getenv("SYNC_FATAL_CHILD") != "1" {
		t.Skip("仅由 TestUnlockUnlockedMutexIsFatal 以子进程拉起")
	}

	var mu sync.Mutex
	mu.Unlock() // fatal: sync: unlock of unlocked mutex
}

// TestRWMutexWriteGate 第 3 节：写者排队后新读者被闸门挡住
func TestRWMutexWriteGate(t *testing.T) {
	var mu sync.RWMutex

	mu.RLock() // 存量读者先到

	writerHeld := make(chan struct{})
	go func() {
		mu.Lock() // 写者到达：readerCount 变负，闸门落下
		close(writerHeld)
		time.Sleep(100 * time.Millisecond)
		mu.Unlock()
	}()
	time.Sleep(50 * time.Millisecond) // 确保写者已排队

	newReader := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		mu.RLock() // 新读者被 readerSem 挡住
		newReader <- time.Since(start)
		mu.RUnlock()
	}()
	time.Sleep(50 * time.Millisecond)

	mu.RUnlock() // 存量读者退出 → 写者拿锁 → 持锁 100ms → 新读者放行
	<-writerHeld
	blocked := <-newReader

	assert.GreaterOrEqual(t, blocked, 30*time.Millisecond, "写者排队后新读者被挡：写者不会饿死")
}

// TestWaitGroupRoundReuse 第 4 节：Add 先于 go，归零后可复用
func TestWaitGroupRoundReuse(t *testing.T) {
	var wg sync.WaitGroup
	var completed atomic.Int64

	for i := 1; i <= 3; i++ {
		wg.Add(1) // 协议：在启动方 Add，不在子 goroutine 里 Add
		go func(id int) {
			defer wg.Done()
			time.Sleep(time.Duration(id) * 5 * time.Millisecond)
			completed.Add(1)
		}(i)
	}
	wg.Wait()
	assert.Equal(t, int64(3), completed.Load(), "第一轮 3 个任务全部完成")

	wg.Add(1)
	go func() { defer wg.Done(); completed.Add(1) }()
	wg.Wait()
	assert.Equal(t, int64(4), completed.Load(), "归零→Wait 返回→可重新 Add 复用")
}

// TestAtomicVsMutexCounting 第 5 节：两条计数路径结果一致
func TestAtomicVsMutexCounting(t *testing.T) {
	const workers = 100
	const per = 1000

	var ac atomic.Int64
	runWorkers(workers, func() {
		for i := 0; i < per; i++ {
			ac.Add(1)
		}
	})

	var mu sync.Mutex
	var mc int64
	runWorkers(workers, func() {
		for i := 0; i < per; i++ {
			mu.Lock()
			mc++
			mu.Unlock()
		}
	})

	assert.Equal(t, int64(workers*per), ac.Load(), "atomic 累加不丢更新")
	assert.Equal(t, int64(workers*per), mc, "mutex 累加同样不丢（快慢对照看 Benchmark）")
}

type configSnapshot struct {
	Timeout time.Duration
	Retries int
}

// TestAtomicPointerConfigSnapshot 第 5 节：整体换快照保证字段一致
func TestAtomicPointerConfigSnapshot(t *testing.T) {
	var cfg atomic.Pointer[configSnapshot]
	cfg.Store(&configSnapshot{Timeout: time.Second, Retries: 3})

	old := cfg.Load()
	cfg.Store(&configSnapshot{Timeout: 2 * time.Second, Retries: 5}) // 整体原子替换
	now := cfg.Load()

	assert.Equal(t, configSnapshot{time.Second, 3}, *old, "换快照前的 Load 仍是旧值")
	assert.Equal(t, configSnapshot{2 * time.Second, 5}, *now, "读端拿到的永远是完整一致的快照")
}

// TestPoolEmptyGetUsesNew 第 6 节：Get 无保证，空池只能靠 New
func TestPoolEmptyGetUsesNew(t *testing.T) {
	var newCalls atomic.Int64
	pool := &sync.Pool{New: func() any {
		newCalls.Add(1)
		return make([]byte, 0, 64)
	}}

	_ = pool.Get().([]byte) // 空池取 → 必然落到 New
	assert.Equal(t, int64(1), newCalls.Load(), "Get 无保证，空池只能靠 New")
}

// TestPoolPutWithoutResetLeaks 第 6 节：Pool 不清零——放回前不 Reset，上一轮的数据就漏给下个使用者
func TestPoolPutWithoutResetLeaks(t *testing.T) {
	pool := &sync.Pool{New: func() any { return make([]byte, 0, 64) }}

	// 单次 Put 后能否取回同一个对象并不确定（别的 P 取走、中途 GC 都会让它退回 New），
	// 所以用 100 轮统计命中次数：实测普通模式 100/100、-race 下也有 70 上下，阈值取 30 留足余量
	leaked := 0
	for i := 0; i < 100; i++ {
		b := pool.Get().([]byte)
		b = append(b, "secret-token"...)
		pool.Put(b) // 不 Reset 直接放回
		if len(pool.Get().([]byte)) > 0 {
			leaked++
		}
	}
	assert.GreaterOrEqual(t, leaked, 30,
		"复用到的对象带着上一轮的残留：Reset 必须由放回方自己做（Pool 不替你清零）")
}

// TestSyncMapLoadOrStoreAndRange 第 7 节：LoadOrStore 只初始化一次，Range 最终一致
func TestSyncMapLoadOrStoreAndRange(t *testing.T) {
	var m sync.Map

	m.Store("route-a", 1)
	m.Store("route-b", 2)

	v, ok := m.Load("route-a")
	assert.True(t, ok, "已存在的键 Load 命中")
	assert.Equal(t, 1, v, "Load 返回已存入的值 1")

	actual, loaded := m.LoadOrStore("route-c", 3)
	assert.Equal(t, 3, actual, "键不存在时写入并返回所提供的值")
	assert.False(t, loaded, "键不存在：写入并返回 false")

	actual, loaded = m.LoadOrStore("route-c", 99)
	assert.Equal(t, 3, actual, "键已存在时不覆盖，返回既有值")
	assert.True(t, loaded, "键已存在：不覆盖")

	m.Delete("route-b")
	_, ok = m.Load("route-b")
	assert.False(t, ok, "键已删除：Load 不再命中")

	count := 0
	m.Range(func(k, v any) bool {
		count++
		return true
	})
	assert.Equal(t, 2, count, "只剩 route-a 与 route-c（sync.Map 没有 len）")
}
