// # sync 锁与原子操作实验
//
// 对应笔记：notes/golang/07-sync锁与原子操作.md
//
// 运行（接入 main.go 后）：
//
//	go run ./experiments/ sync
//
// 实验项：
//
//	第1节：Mutex 快路径无竞争 + Unlock 无属主校验（他人解锁合法）
//	第2节：RWMutex 写锁等待期间新读者被阻塞（readerCount 闸门）
//	第3节：WaitGroup 协议与复用窗口
//	第4节：atomic CAS 无锁计数 vs Mutex 计数吞吐对比 + atomic.Pointer 配置快照
//	第5节：sync.Pool GC 清空与放回前 Reset
//	第6节：sync.Map 读路径与 Range 最终一致语义
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// RunSyncExperiments 演示笔记 7 的锁与原子操作行为。
func RunSyncExperiments() {
	fmt.Println("========== 第1节: Mutex 基本语义与无属主校验 ==========")
	syncMutex()

	fmt.Println("\n========== 第2节: RWMutex 写锁闸门 ==========")
	syncRWMutex()

	fmt.Println("\n========== 第3节: WaitGroup 协议与复用 ==========")
	syncWaitGroup()

	fmt.Println("\n========== 第4节: atomic vs Mutex 计数 + 配置快照 ==========")
	syncAtomic()

	fmt.Println("\n========== 第5节: sync.Pool 生命周期 ==========")
	syncPool()

	fmt.Println("\n========== 第6节: sync.Map 语义 ==========")
	syncMap()
}

// syncMutex 第1节：Mutex 的三个可观察语义。
func syncMutex() {
	var mu sync.Mutex

	// 1) 无竞争时加锁/解锁就是一次 CAS，纳秒级
	start := time.Now()
	for i := 0; i < 1000000; i++ {
		mu.Lock()
		mu.Unlock()
	}
	fmt.Printf("无竞争 Lock/Unlock ×100万: %v（快路径 = 一次 CAS，不进调度器）\n", time.Since(start))

	// 2) Unlock 不校验属主：goroutine A 加锁、主 goroutine 解锁，合法
	mu.Lock()
	done := make(chan struct{})
	go func() {
		mu.Unlock() // 他人解锁：Mutex 不记录谁加的锁
		close(done)
	}()
	<-done
	mu.Lock() // 解锁后可正常再次加锁
	fmt.Println("他人 Unlock: 合法（Mutex 无属主）—— 但工程上禁止，review 打回")

	// 3) 未加锁就 Unlock 是 fatal（不可 recover），这里只打印说明不真跑
	fmt.Println("未加锁 Unlock: fatal panic 且不可 recover（sync: unlock of unlocked mutex）")

	mu.Unlock()
}

// syncRWMutex 第2节：写锁等待时新读者被阻塞（写者不会饿死）。
func syncRWMutex() {
	var mu sync.RWMutex

	mu.RLock() // 存量读者先到

	writerHeld := make(chan struct{})
	go func() {
		mu.Lock() // 写者到达，等存量读者退出（readerCount 打为负）
		writerHeld <- struct{}{}
		time.Sleep(100 * time.Millisecond) // 持锁一段时间，放大观察窗口
		mu.Unlock() // 写者离场，被闸门挡住的新读者随之放行
	}()
	time.Sleep(100 * time.Millisecond) // 确保写者已排队

	newReaderDone := make(chan time.Duration)
	go func() {
		start := time.Now()
		mu.RLock() // 新读者：写者在排队 → 被 readerSem 挡住
		newReaderDone <- time.Since(start)
		mu.RUnlock()
	}()
	time.Sleep(100 * time.Millisecond) // 确保新读者已阻塞在 RLock 上

	mu.RUnlock() // 存量读者退出 → 写者拿锁、持锁 100ms → 离场 → 新读者放行
	<-writerHeld
	blocked := <-newReaderDone

	fmt.Printf("写者排队后，新读者 RLock 被挡了 %v（排队等待 + 写者持锁，直到写者离场才放行）\n", blocked.Round(10*time.Millisecond))
	fmt.Println("原理: Lock 时 readerCount 减一个大常数变负，此后 RLock 见负数即睡 readerSem")
	fmt.Println("推论: 读锁不能升级（持读锁再 Lock 死锁），可以降级（Lock→RLock→RUnlock→Unlock）")
}

// syncWaitGroup 第3节：Add 必须先于 go；计数归零后可复用。
func syncWaitGroup() {
	var wg sync.WaitGroup

	for i := 1; i <= 3; i++ {
		wg.Add(1) // 协议：在启动方 Add，不在子 goroutine 里 Add
		go func(id int) {
			defer wg.Done()
			time.Sleep(time.Duration(id) * 10 * time.Millisecond)
		}(i)
	}
	wg.Wait()
	fmt.Println("第一轮: 3 个任务全部完成")

	// 复用：Wait 返回后重新 Add 一轮（不要在 Wait 未返回时并发 Add）
	wg.Add(1)
	go func() { defer wg.Done() }()
	wg.Wait()
	fmt.Println("第二轮: 复用 wg 成功（归零→Wait 返回→重新 Add）")

	fmt.Println("两条 fatal: ①Wait 后并发 Add（misuse）②Done 多于 Add（负计数），均不可 recover")
	fmt.Println("正解: 需要超时/错误收集/并发上限时用 errgroup.WithContext + SetLimit")
}

// syncAtomic 第4节：单字保护用 atomic；配置快照用 atomic.Pointer 整体换。
func syncAtomic() {
	const n = 1000
	const per = 10000

	// 1) atomic 计数
	var ac atomic.Int64
	start := time.Now()
	runWorkers(n, func() {
		for i := 0; i < per; i++ {
			ac.Add(1)
		}
	})
	atomicDur := time.Since(start)

	// 2) Mutex 计数
	var mu sync.Mutex
	var mc int64
	start = time.Now()
	runWorkers(n, func() {
		for i := 0; i < per; i++ {
			mu.Lock()
			mc++
			mu.Unlock()
		}
	})
	mutexDur := time.Since(start)

	fmt.Printf("1000 goroutine × 1万次计数: atomic=%v mutex=%v（atomic 无锁更快，但高竞争下都退化为串行）\n", atomicDur, mutexDur)
	fmt.Printf("结果一致: atomic=%d mutex=%d\n", ac.Load(), mc)

	// 3) atomic.Pointer 配置热更新：整体换快照，字段间天然一致
	type config struct {
		Timeout time.Duration
		Retries int
	}
	var cfg atomic.Pointer[config]
	cfg.Store(&config{Timeout: time.Second, Retries: 3})

	old := cfg.Load()
	cfg.Store(&config{Timeout: 2 * time.Second, Retries: 5}) // 原子替换
	now := cfg.Load()
	fmt.Printf("配置快照: 旧=%v/重试%d 新=%v/重试%d（读端拿到的永远是完整一致的快照）\n",
		old.Timeout, old.Retries, now.Timeout, now.Retries)

	fmt.Println("边界: atomic 只保护一个字；两个 int 组成的二元状态必须打包或上锁；无原子浮点 Add")
}

// runWorkers 启动 n 个 worker 跑 fn。
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

// syncPool 第5节：Pool 语义 = 缓存而非容器；GC 清空；放回前必须 Reset。
func syncPool() {
	pool := &sync.Pool{
		New: func() any { return new(sync.Map) }, // New 必须提供：Get 无保证
	}

	a := pool.Get()
	pool.Put(a)
	b := pool.Get()
	fmt.Printf("Put 后立刻 Get: %v（可能命中同一对象，也可能没有——无保证）\n", a == b)

	// GC 后清空：对象可能被丢弃，再 Get 走 New
	runtime.GC()
	runtime.GC() // 两次：主代降级 victim 后，再一次才真正丢弃
	c := pool.Get()
	fmt.Printf("两次 GC 后 Get: %v（新对象，证明 Pool 不是缓存/容器）\n", a != c || b != c)

	// 脏数据事故演示：放回前不 Reset，下个使用者读到残留
	bufPool := &sync.Pool{
		New: func() any { return make([]byte, 0, 64) },
	}
	dirty := bufPool.Get().([]byte)
	dirty = append(dirty, "secret-token"...)
	bufPool.Put(dirty) // ✗ 没 Reset 就放回
	next := bufPool.Get().([]byte)
	fmt.Printf("未 Reset 放回: 下个使用者拿到 %q（状态残留 = 最高频事故）\n", string(next))

	fmt.Println("定位: 减 GC 压力（bytes.Buffer/大对象复用），不是缓存/连接池/free list")
}

// syncMap 第6节：读路径与 Range 的最终一致。
func syncMap() {
	var m sync.Map

	m.Store("route-a", 1)
	m.Store("route-b", 2)

	// Load：读多场景零锁快路径（Go 1.24+ HashTrieMap，读无锁）
	v, ok := m.Load("route-a")
	fmt.Printf("Load 命中: %v %v\n", v, ok)

	// LoadOrStore：并发"只初始化一次"的惯用法
	actual, loaded := m.LoadOrStore("route-c", 3)
	fmt.Printf("LoadOrStore: 值=%v 已存在=%v\n", actual, loaded)

	// Delete 是惰性的，Range 最终一致
	m.Delete("route-b")
	count := 0
	m.Range(func(k, v any) bool {
		count++
		return true
	})
	fmt.Printf("Range 遍历到 %d 个 key（sync.Map 没有 len()）\n", count)

	fmt.Println("适用: ①key 写一次读多次且集合稳定 ②多 goroutine 写不相干的 key")
	fmt.Println("反场景: 持续写同一批 key / 需要精确快照 → map+Mutex 更好")
}
