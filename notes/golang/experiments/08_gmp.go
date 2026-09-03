// # 运行时调度器 GMP 实验
//
// 对应笔记：notes/golang/08-运行时调度器GMP.md
//
// 运行（接入 main.go 后）：
//
//	go run ./experiments/ gmp
//
// 实验项：
//
//	第1节：GOMAXPROCS / NumCPU / NumGoroutine 基本观察
//	第2节：批量启动 goroutine，观察 NumGoroutine 增长与回落
//	第3节：runtime.Gosched() 主动让出（单 P 下对比）
//	第4节：纯 CPU 循环与信号异步抢占（Go 1.14+ 不再饿死其他 G）
//	第5节：channel 阻塞不占线程 vs 阻塞系统调用占用 M（hand off 的直接观测）
package main

import (
	"fmt"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"
	"time"
)

// RunGMPExperiments 演示笔记 8 的 GMP 调度行为。
func RunGMPExperiments() {
	fmt.Println("========== 第1节: GOMAXPROCS / NumCPU / NumGoroutine ==========")
	gmpBasic()

	fmt.Println("\n========== 第2节: goroutine 数量增长与回落 ==========")
	gmpGrowth()

	fmt.Println("\n========== 第3节: runtime.Gosched() 主动让出 ==========")
	gmpGosched()

	fmt.Println("\n========== 第4节: 纯 CPU 循环与信号异步抢占 ==========")
	gmpPreempt()

	fmt.Println("\n========== 第5节: channel 阻塞 vs 系统调用阻塞 ==========")
	gmpBlockCompare()
}

// gmpBasic 第1节：P 的数量语义。P = GOMAXPROCS，M 按需创建。
func gmpBasic() {
	fmt.Printf("runtime.NumCPU()      = %d（逻辑 CPU 数）\n", runtime.NumCPU())
	fmt.Printf("runtime.GOMAXPROCS(0) = %d（P 的数量，默认 = NumCPU）\n", runtime.GOMAXPROCS(0))
	fmt.Printf("runtime.NumGoroutine()= %d（当前存活 G 数）\n", runtime.NumGoroutine())

	prev := runtime.GOMAXPROCS(2) // 返回修改前的旧值
	fmt.Printf("调用 GOMAXPROCS(2) 后：P = %d（旧值 = %d），已恢复\n", runtime.GOMAXPROCS(0), prev)
	runtime.GOMAXPROCS(prev)

	fmt.Println("注意：M 数量 ≠ P —— M 按需创建，默认上限 10000（debug.SetMaxThreads 可调）")
	fmt.Println("（Go 1.25+ 在 Linux 下 GOMAXPROCS 会自动感知 cgroup CPU limit 并动态更新）")
}

// gmpGrowth 第2节：goroutine 很便宜，千级共存毫不费力。
func gmpGrowth() {
	fmt.Printf("启动前 NumGoroutine = %d\n", runtime.NumGoroutine())

	done := make(chan struct{})
	for i := 0; i < 1000; i++ {
		go func() { <-done }() // 全部阻塞在 channel 上（G park，不占线程）
	}

	for i := 1; i <= 5; i++ {
		time.Sleep(20 * time.Millisecond) // 给调度器时间把它们跑起来
		fmt.Printf("t=%3dms NumGoroutine = %d\n", i*20, runtime.NumGoroutine())
	}

	close(done) // 广播退出
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("全部结束后 NumGoroutine = %d（回到基线附近）\n", runtime.NumGoroutine())
	fmt.Println("（千级 goroutine 秒建秒回收：2KB 初始栈 + 用户态调度，见笔记 §7）")
}

// gmpGosched 第3节：Gosched 提供确定性的立即让出；抢占只是兜底。
func gmpGosched() {
	prev := runtime.GOMAXPROCS(1) // 单 P 下让出效果最直观
	defer runtime.GOMAXPROCS(prev)

	var wg sync.WaitGroup

	wg.Add(2)
	go gmpPrintLetter(&wg, "A")
	go gmpPrintLetter(&wg, "B")
	wg.Wait()
	fmt.Println("  <- 不 Gosched：可能成块也可能交错（fmt.Print 是函数调用=协作抢占点，但时机不确定）")

	wg.Add(2)
	go gmpYieldLetter(&wg, "A")
	go gmpYieldLetter(&wg, "B")
	wg.Wait()
	fmt.Println("  <- Gosched：基本确定性交替；不必等 10ms 抢占兜底（Go 1.14+，尾部乱序来自写 stdout 的系统调用）")
}

// gmpPrintLetter 不让出，全靠抢占点。
func gmpPrintLetter(wg *sync.WaitGroup, s string) {
	defer wg.Done()
	for i := 0; i < 6; i++ {
		fmt.Print(s)
	}
}

// gmpYieldLetter 每次输出前主动让出：G 放回运行队列尾部，M 立刻取下一个 G。
func gmpYieldLetter(wg *sync.WaitGroup, s string) {
	defer wg.Done()
	for i := 0; i < 6; i++ {
		runtime.Gosched()
		fmt.Print(s)
	}
}

// gmpPreempt 第4节：无函数调用的纯 CPU 循环，Go 1.14+ 靠信号（SIGURG）抢占。
func gmpPreempt() {
	prev := runtime.GOMAXPROCS(1) // 单 P：若抢占失效，tick 会被饿死
	defer runtime.GOMAXPROCS(prev)

	done := make(chan int64)
	go func() {
		var sum int64 // 循环体内零函数调用 → 不是协作式抢占点（Go 1.13 前的盲区）
		for i := int64(0); i < 1<<31; i++ {
			sum += i
		}
		done <- sum
	}()

	t0 := time.Now()
	for i := 1; i <= 5; i++ {
		time.Sleep(50 * time.Millisecond)
		fmt.Printf("tick %d @ %6v —— sleep 醒来的主 goroutine 依然能被调度\n", i, time.Since(t0).Round(time.Millisecond))
	}

	sum := <-done
	fmt.Printf("纯 CPU goroutine 完成（sum=%d）：同 P 的其他 G 没被饿死\n", sum)
	fmt.Println("机制：sysmon 发现 G 连续运行超 10ms → 向 M 发 SIGURG → asyncPreempt 强制进入调度器（Go 1.14+）")
}

// gmpBlockCompare 第5节：channel 阻塞挂起 G 不占线程；系统调用阻塞占用 M、P 被 hand off。
func gmpBlockCompare() {
	threads := func() int { return pprof.Lookup("threadcreate").Count() }

	// A：100 个 goroutine 阻塞在 channel 上 —— G 被 park，M/P 不受影响
	base := threads()
	parked := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-parked }()
	}
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("100 个 G 阻塞在 channel 上：线程数 %d -> %d（几乎不变，G 挂起即可）\n", base, threads())
	close(parked)
	wg.Wait()

	// B：16 个 goroutine 阻塞在系统调用上 —— 每个都占住一个 M（陪绑在内核里）。
	// 数量超过当前线程总数，idle 池不够用，runtime 只能新建 M 接管被 hand off 的 P
	base = threads()
	tv := syscall.Timeval{Sec: 1} // 睡 1 秒的 select，可自行结束
	var wg2 sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			syscall.Select(0, nil, nil, nil, &tv) // 直接陷入内核的阻塞系统调用
		}()
	}
	time.Sleep(300 * time.Millisecond)
	fmt.Printf("  16 个 G 阻塞在系统调用上：线程数 %d -> %d（每个阻塞的 G 都占一个 M，runtime 只能新建 M）\n", base, threads())
	wg2.Wait()

	fmt.Println("（对比结论：channel 阻塞 park 的是 G；系统调用阻塞陪绑的是 M —— 两条路径分离，见笔记 §3/§8-Q2）")
	fmt.Println("（数量少于现有线程数时 runtime 复用 idle M，不一定新建；超过则必然增长）")
}
