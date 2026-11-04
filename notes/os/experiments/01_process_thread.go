// # 协程与线程实验（笔记 01）
//
// 对应笔记：notes/os/01-进程与线程.md
//
// 运行：go run ./experiments/ threads
package main

import (
	"fmt"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

// ExpThreads 观察「协程数暴涨」与「内核线程数」的关系：
// goroutine 阻塞在 channel 上只占用用户态栈，不会为它创建内核线程。
func ExpThreads() {
	fmt.Printf("GOMAXPROCS=%d  NumCPU=%d\n", runtime.GOMAXPROCS(0), runtime.NumCPU())
	fmt.Printf("起始：NumGoroutine=%d 累计创建的内核线程=%d\n", runtime.NumGoroutine(), threadCount())

	const n = 10000
	block := make(chan struct{})
	var wg sync.WaitGroup

	start := time.Now()
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-block // 阻塞在 channel 上：用户态等待，不占内核线程
		}()
	}
	time.Sleep(300 * time.Millisecond)

	fmt.Printf("起 %d 个阻塞在 channel 的 goroutine（%v 内建完）：NumGoroutine=%d 累计内核线程=%d\n",
		n, time.Since(start).Round(time.Millisecond),
		runtime.NumGoroutine(), threadCount())

	close(block)
	wg.Wait()
	time.Sleep(100 * time.Millisecond) // 让 runtime 回收协程栈，读数稳定
	fmt.Printf("1 万个协程退出后：NumGoroutine=%d 累计内核线程=%d（残留的是 runtime 后台协程）\n",
		runtime.NumGoroutine(), threadCount())

	fmt.Println()
	fmt.Println("判读：")
	fmt.Println("  NumGoroutine 从个位数涨到 1 万，而 threadcreate 计数几乎不动——")
	fmt.Println("  说明这 1 万个协程并没有变成 1 万个内核线程：它们只是各自 2KB 起的用户态栈。")
	fmt.Println("  threadcreate 计数只会在 runtime 需要新 M 时才增长，典型触发是「阻塞系统调用」")
	fmt.Println("  （文件 IO、cgo 调用、无超时的 syscall）——那也是线程数失控的唯一入口。")
}

// threadCount 返回 runtime 累计创建的内核线程数（M 的近似值）。
func threadCount() int {
	if p := pprof.Lookup("threadcreate"); p != nil {
		return p.Count()
	}
	return -1
}
