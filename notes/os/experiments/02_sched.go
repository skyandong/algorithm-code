// # 并行加速比实验（笔记 02）
//
// 对应笔记：notes/os/02-CPU调度与cgroup.md
//
// 运行：go run ./experiments/ sched
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const totalIters = 200_000_000

// sink 防止编译器把计算优化掉
var sink int64

// ExpSched 对比「单线程跑完总量」与「N 个线程分担同样总量」的墙钟时间。
func ExpSched() {
	cores := runtime.GOMAXPROCS(0)

	seq := runBatch(1)
	fmt.Printf("① 单 goroutine 跑 %d 次迭代：墙钟 %v\n", totalIters, seq.Round(time.Millisecond))

	par := runBatch(cores)
	fmt.Printf("② %d 个 goroutine 分担同样总量：墙钟 %v\n", cores, par.Round(time.Millisecond))

	speedup := float64(seq) / float64(par)
	fmt.Printf("③ 加速比 %.2fx（理论上限 %dx）\n", speedup, cores)

	ru := rusage()
	fmt.Printf("   进程累计 CPU：user=%v sys=%v\n",
		time.Duration(ru.Utime.Sec)*time.Second+time.Duration(ru.Utime.Usec)*time.Microsecond,
		time.Duration(ru.Stime.Sec)*time.Second+time.Duration(ru.Stime.Usec)*time.Microsecond)

	fmt.Println()
	fmt.Println("判读：")
	fmt.Printf("  加速比明显低于 %dx 时，通常只有三种原因：\n", cores)
	fmt.Println("    1. 机器实际可用的 CPU 少于 NumCPU（容器 cpu.max 配额小于核数）")
	fmt.Println("    2. 有串行段（阿姆达尔定律）或锁争抢")
	fmt.Println("    3. 内存带宽/NUMA 成为瓶颈，加线程不再变快")
	fmt.Println("  容器里要确认 cpu.max：cat /sys/fs/cgroup/cpu.max（Linux）")
}

// runBatch 用 workers 个 goroutine 分担 totalIters 次迭代，返回墙钟时间。
func runBatch(workers int) time.Duration {
	if workers < 1 {
		workers = 1
	}
	per := totalIters / workers
	var wg sync.WaitGroup
	start := time.Now()
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			x := 0
			for j := 0; j < per; j++ {
				x += j % 7
			}
			atomic.AddInt64(&sink, int64(x))
		}()
	}
	wg.Wait()
	return time.Since(start)
}

// rusage 读取进程的 CPU 时间累计值。
func rusage() syscall.Rusage {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return ru
}
