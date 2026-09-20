// 实验 06（sync 锁与原子操作），断言验证
// 对应笔记：notes/golang/06-sync并发原语.md
// 源码对照：src/internal/sync/mutex.go（sync.Mutex 本体在 internal/sync，src/sync/mutex.go 是薄包装）
// 源码对照：src/internal/sync/hashtriemap.go（sync.Map 本体是 HashTrieMap 的薄包装）
// 源码对照：src/sync/once.go（Once.Do 的快路径是 atomic.Load 检查）
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

// TestMutexNotReentrant 第 1 节：Mutex 不记录属主、也不可重入——持锁者再取同一把锁必失败
// （常见错例：A 持锁后调用 C，C 又 Lock 同一把锁 → C 永久阻塞、A 的 defer 永远执行不到）
func TestMutexNotReentrant(t *testing.T) {
	var mu sync.Mutex

	mu.Lock()
	assert.False(t, mu.TryLock(), "已持有的锁不可重入：TryLock 直接失败（换成 Lock 就是永久阻塞）")
	mu.Unlock()

	assert.True(t, mu.TryLock(), "释放后同一把锁可再取")
	mu.Unlock()
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

// TestPoolKeepsGrownBuffer 第 6 节：池按「整个对象」存取，放回的大对象不会自动缩容
func TestPoolKeepsGrownBuffer(t *testing.T) {
	pool := &sync.Pool{New: func() any { return make([]byte, 0, 16) }}

	big := pool.Get().([]byte)
	big = append(big, make([]byte, 64*1024)...) // 撑到 64KB
	pool.Put(big)

	assert.GreaterOrEqual(t, cap(pool.Get().([]byte)), 64*1024,
		"容量不会因为下次只需要一点点就缩回去——池里混进大对象就是长期驻留，放回前要按容量筛")
}

// TestSyncMapLoadOrStoreAndRange 第 7 节：LoadOrStore 只初始化一次，Range 最终一致
func TestSyncMapLoadOrStoreAndRange(t *testing.T) {
	var m sync.Map

	_, ok := m.Load("k")
	assert.False(t, ok, "未写入的键：ok=false（与「值为零值」区分开）")

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

	// 并发读写本身没有可确定的断言（谁最后写不可知），价值在 -race 下验证无数据竞争
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				m.Store(i%16, i)
				m.Load(i % 16)
			}
		}()
	}
	wg.Wait()

	_, ok = m.Load(15)
	assert.True(t, ok, "并发写过的 16 个键都能查到")
}

// TestOnceRunsExactlyOnce 第 8 节：sync.Once 只执行一次，完成后对所有人可见
func TestOnceRunsExactlyOnce(t *testing.T) {
	var once sync.Once
	var executed atomic.Int64

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			once.Do(func() {
				executed.Add(1)
				time.Sleep(50 * time.Millisecond)
			})
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), executed.Load(), "sync.Once 只执行一次，完成后对所有人可见")
}

// ===== 实验实现：IP 限流与「带超时的 WaitGroup」（第 1、4 节）=====

// Ban 高并发 IP 限流：三分钟窗口内同 IP 只放行一次（注入时间，完全确定性）。
type Ban struct {
	mu       sync.Mutex
	visitIPs map[string]time.Time
}

// NewBan 构造 IP 限流器（单位时间窗内限次）
func NewBan() *Ban {
	return &Ban{visitIPs: make(map[string]time.Time)}
}

// visit 记一次访问，超过窗口配额返回 false
func (b *Ban) visit(ip string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if last, ok := b.visitIPs[ip]; ok && now.Sub(last) < 3*time.Minute {
		return true
	}
	b.visitIPs[ip] = now
	return false
}

// WaitTimeout 给 WaitGroup 加超时：超时返回 true（调用方负责取消），自然结束返回 false。
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return false
	case <-timer.C:
		return true
	}
}

// ===== 断言用例 =====

// TestBanVisit 第 1 节：三分钟窗口内同 IP 只放行一次（注入时间，完全确定性）。
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

// TestBanVisitConcurrency 第 1 节：100 个不同 IP 并发各访一次，恰好放行 100 次（无竞态）。
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

// TestWaitTimeout 第 4 节：超时返回 true（调用方负责取消）、自然结束返回 false。
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
	assert.True(t, WaitTimeout(&wg, 100*time.Millisecond), "未结束的 WaitGroup 触发超时返回 true")
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
	assert.False(t, WaitTimeout(&wg2, 2*time.Second), "协程自然结束返回 false（未超时）")
}

// TestOnceReturnsOnlyAfterInit 第 8 节：Do 返回时初始化必然已完成
// （手写双检查把完成标志设在 f() 之前，就退化成「标志 = 已开始」——别的调用者会拿到未完成的初始化结果）
func TestOnceReturnsOnlyAfterInit(t *testing.T) {
	var once sync.Once
	var initDone atomic.Bool

	go once.Do(func() {
		time.Sleep(30 * time.Millisecond) // 慢初始化
		initDone.Store(true)
	})

	time.Sleep(10 * time.Millisecond) // 让第一个调用者先进入 Do
	once.Do(func() {})                // 第二个调用者必须等，而不是拿到「已开始」就返回

	assert.True(t, initDone.Load(), "第二个调用者从 Do 返回时，初始化已完成（不是「已开始」）")
}

// BenchmarkSyncMapRead 读路径基准：读多写少场景 sync.Map 胜出
func BenchmarkSyncMapRead(b *testing.B) {
	var sm sync.Map
	for i := 0; i < 16; i++ {
		sm.Store(i, i)
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sm.Load(1)
		}
	})
}

// BenchmarkRWMutexMapRead 读路径对照基准：map + RWMutex
func BenchmarkRWMutexMapRead(b *testing.B) {
	m := map[int]int{1: 1}
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			_ = m[1]
			mu.RUnlock()
		}
	})
}
