// # 内存可见性与 sync.Once 实验
//
// 对应笔记：notes/golang/04-并发内存可见性与sync.Once.md
//
// 运行：
//
//	go run ./experiments/ visibility
//	go run -race ./experiments/ visibility   // 观察 data race 报告
//
// 实验项：
//
//	Exp1：data race 演示 — 普通变量跨 goroutine 传递状态不可依赖
//	Exp2：atomic 发布状态 — Store/Load 建立 happens-before
//	Exp3：channel 发布 — close(done) 通知完成
//	Exp4：mutex 建立同步关系 — Lock/Unlock 同时保证可见性
//	Exp5：错误的双重检查 Once — 外层无锁读 done 是 data race
//	Exp6：正确的 sync.Once — 初始化只执行一次
//
// —— runtime 源码对照 ——
//
// src/sync/once.go（Do 的核心路径，逐行对应源码）
//
//	type Once struct {
//		_    noCopy
//		done atomic.Bool // done 放首字段：热路径内联时寻址更紧凑
//		m    Mutex
//	}
//
//	func (o *Once) Do(f func()) {
//		if !o.done.Load() { // 快路径：一次原子读，无锁
//			o.doSlow(f)
//		}
//	}
//
//	func (o *Once) doSlow(f func()) {
//		o.m.Lock()
//		defer o.m.Unlock()
//		if !o.done.Load() { // 双检查：拿到锁后必须再查一次
//			defer o.done.Store(true) // Store 延迟到 f 返回后——保证 Do 返回时 f 已完成
//			f()
//		}
//	}
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// RunVisibilityExperiments 演示笔记 5 的核心概念。
func RunVisibilityExperiments() {
	fmt.Println("===== 1. data race 演示（强烈建议用 go run -race 运行本命令）=====")
	demoDataRace()

	fmt.Println("\n===== 2. atomic 发布状态 =====")
	demoAtomicPublish()

	fmt.Println("\n===== 3. channel 发布（close 通知）=====")
	demoChannelPublish()

	fmt.Println("\n===== 4. mutex 建立同步关系 =====")
	demoMutexSync()

	fmt.Println("\n===== 5. 错误的双重检查 Once（无锁读 done 是 data race）=====")
	demoWrongOnce()

	fmt.Println("\n===== 6. 正确的 sync.Once =====")
	demoCorrectOnce()

	fmt.Println("\n===== 附: happens-before 规则速览与 race detector（笔记 05 第 2/3/9/10 节）=====")
	demoHappensBeforeNotes()
}

// demoHappensBeforeNotes 笔记 05 第 2/3/9/10 节：同步原语建立 happens-before；-race 用矢量时钟检测违规。
func demoHappensBeforeNotes() {
	fmt.Println("happens-before 建立边（后一个操作可见前一个的写入）:")
	fmt.Println("  ① go 语句启动新 goroutine：go 之前的写 → 子 goroutine 全部可见")
	fmt.Println("  ② channel：第 n 次发送 happens-before 对应接收完成；close → 零值接收")
	fmt.Println("  ③ Mutex/RWMutex：第 n 次 Unlock → 第 n+1 次 Lock 返回后的读全可见")
	fmt.Println("  ④ sync.Once：Once.Do(f) 返回 → f 里的写对一切调用者可见")
	fmt.Println("  ⑤ atomic：顺序一致性（SeqCst），读写构成全序（类型化 API 推荐）")
	fmt.Println("  ⑥ WaitGroup：Done → Wait 返回")
	fmt.Println()
	fmt.Println("为什么裸读写不可靠（第 2/3 节）: 编译器重排 + CPU 缓存/写缓冲 —— 上面的 demoDataRace")
	fmt.Println("没有同步边，reader 可能永远看到 ready=0；「碰巧对」不代表下次对，靠猜不如建边")
	fmt.Println()
	fmt.Println("race detector（第 10 节）: go run/build/test -race，基于矢量时钟的 happens-before 检测")
	fmt.Println("  - 报告「无同步边的并发访问同一内存」，误报几乎为零，但可能漏报（那段调度没跑到）")
	fmt.Println("  - 代价: 内存 ×5~10、CPU ×2~20 → 只用于测试/CI，生产构建不开")
	fmt.Println("  - 检测的是数据竞争本身，不是竞争导致的错误值——竞争是因，诡异输出是果")
	fmt.Println()
	fmt.Println("机制对比速览（第 9 节）: mutex=互斥+可见 / atomic=单字无锁 / channel=传递数据顺带同步 / Once=单次初始化")
}

// demoDataRace 复现笔记 5 第 1 节：writer 写 data/ready，reader 忙等 ready。
// 这里用有界循环 + 截止时间避免无限自旋；普通变量跨 goroutine 读写构成 data race，
// 结果不可预测 —— 用 -race 运行可看到 "WARNING: DATA RACE"。
func demoDataRace() {
	var data int
	var ready int

	go func() {
		data = 42
		ready = 1
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for ready == 0 {
		if time.Now().After(deadline) {
			fmt.Println("reader: 等待超时（没有同步关系，不能依赖观察顺序）")
			return
		}
		runtime.Gosched()
	}
	fmt.Println("reader 读到 data =", data, "（结果不确定，-race 下会报 DATA RACE）")
}

// demoAtomicPublish 笔记 5 第 4 节：atomic Store/Load 发布状态。
func demoAtomicPublish() {
	var data atomic.Int64
	var ready atomic.Bool

	go func() {
		data.Store(42)
		ready.Store(true)
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for !ready.Load() {
		if time.Now().After(deadline) {
			fmt.Println("atomic: 等待超时")
			return
		}
		runtime.Gosched()
	}
	fmt.Println("atomic: data =", data.Load(), "（Store 先于 ready.Store，Load 顺序保证可见）")
}

// demoChannelPublish 笔记 5 第 4 节：close(done) 发布 —— 关闭前的写入对收到通知的代码可见。
func demoChannelPublish() {
	done := make(chan struct{})
	var data int

	go func() {
		data = 42
		close(done)
	}()

	<-done
	fmt.Println("channel: data =", data, "（close 前的写入 happens-before 收到通知）")
}

// demoMutexSync 笔记 5 第 5 节：mutex 不只是互斥，还建立可见性。
func demoMutexSync() {
	var mu sync.Mutex
	var data int
	var ready bool

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu.Lock()
		data = 42
		ready = true
		mu.Unlock()
	}()

	// 这里必须也加锁读，否则仍与写入构成 data race（笔记强调的点）
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if ready {
		fmt.Println("mutex: data =", data, "（Unlock happens-before 之后的 Lock）")
	}
}

// wrongOnce 复现笔记 5 第 6 节：外层无锁读 done，是错误实现。
type wrongOnce struct {
	mu   sync.Mutex
	done uint32
}

func (o *wrongOnce) Do(f func()) {
	if o.done == 1 { // 无锁读，与下面的写构成 data race
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.done == 0 {
		o.done = 1 // 且在 f 之前设置，"完成"语义错误
		f()
	}
}

// demoWrongOnce 用 -race 运行可看到外层读取与外层写入的竞争。
func demoWrongOnce() {
	var once wrongOnce
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
	fmt.Println("wrongOnce: executed =", executed.Load(), "（race 存在，结果不可依赖；-race 会报告）")
}

// demoCorrectOnce 笔记 5 第 7 节：直接用标准库 sync.Once。
func demoCorrectOnce() {
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
	fmt.Println("sync.Once: executed =", executed.Load(), "（初始化只执行一次，且完成后对所有人可见）")
}
