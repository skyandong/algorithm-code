// # 内存可见性与 sync.Once 实验
//
// 对应笔记：notes/golang/01-并发内存可见性与sync.Once.md
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
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// RunVisibilityExperiments 演示笔记 1 的核心概念。
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
}

// demoDataRace 复现笔记 1 第 1 节：writer 写 data/ready，reader 忙等 ready。
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

// demoAtomicPublish 笔记 1 第 4 节：atomic Store/Load 发布状态。
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

// demoChannelPublish 笔记 1 第 4 节：close(done) 发布 —— 关闭前的写入对收到通知的代码可见。
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

// demoMutexSync 笔记 1 第 5 节：mutex 不只是互斥，还建立可见性。
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

// wrongOnce 复现笔记 1 第 6 节：外层无锁读 done，是错误实现。
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

// demoCorrectOnce 笔记 1 第 7 节：直接用标准库 sync.Once。
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
