// # 名家博客并发模式实验
//
// 对应笔记：notes/golang/13-名家并发模式汇总.md
//
// 运行：
//
//	go run ./experiments/ masters
//
// 实验项：
//
//	Exp1：Dave Cheney — close(ch) 广播停止信号给 N 个 goroutine
//	Exp2：Dave Cheney — nil channel 优雅多路等待（禁用已关闭的分支）
//	Exp3：William Kennedy — GOMAXPROCS=1 并发但非并行
//	Exp4：谢孟军 — goroutine 基础（Gosched 让出时间片）
//	Exp5：谢孟军 — channel 求和
//	Exp6：谢孟军 — Fibonacci + close + range
//	Exp7：Go 官方博客 — Pipeline 模式 gen→sq→sq
//	Exp8：select + 超时模式
package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// RunMastersExperiments 演示笔记 13 的实战模式。
func RunMastersExperiments() {
	fmt.Println("===== 1. Dave Cheney：close(ch) 广播停止信号 =====")
	mastersBroadcastStop()

	fmt.Println("\n===== 2. Dave Cheney：nil channel 优雅多路等待 =====")
	mastersWaitMany()

	fmt.Println("\n===== 3. William Kennedy：GOMAXPROCS=1 并发非并行 =====")
	mastersGOMAXPROCS1()

	fmt.Println("\n===== 4. 谢孟军：goroutine 基础（Gosched）=====")
	mastersGoschedSay()

	fmt.Println("\n===== 5. 谢孟军：channel 求和 =====")
	mastersChannelSum()

	fmt.Println("\n===== 6. 谢孟军：Fibonacci + close + range =====")
	mastersFibonacci()

	fmt.Println("\n===== 7. Go 官方博客：Pipeline gen→sq→sq =====")
	mastersPipeline()

	fmt.Println("\n===== 8. select + 超时模式 =====")
	mastersSelectTimeout()
}

// mastersBroadcastStop 笔记 13 §1.2：close(finish) 一次广播，N 个 goroutine 同时收到。
func mastersBroadcastStop() {
	const n = 100
	finish := make(chan struct{})
	var done sync.WaitGroup

	for i := 0; i < n; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			select {
			case <-time.After(1 * time.Hour):
			case <-finish:
			}
		}()
	}

	t0 := time.Now()
	close(finish)
	done.Wait()
	fmt.Printf("Waited %v for %d goroutines to stop\n", time.Since(t0), n)
}

// mastersWaitMany 笔记 13 §1.2：nil channel 禁用已关闭分支，避免死循环。
func mastersWaitMany() {
	waitMany := func(a, b chan bool) {
		for a != nil || b != nil {
			select {
			case <-a:
				a = nil // 设为 nil 后该 case 被忽略
			case <-b:
				b = nil
			}
		}
		fmt.Println("两个 channel 都已关闭并消费完，正常退出")
	}

	a, b := make(chan bool), make(chan bool)
	go func() {
		close(a)
		close(b)
	}()
	waitMany(a, b)
}

// mastersGOMAXPROCS1 笔记 13 §2.1：单核上并发执行但非并行。
func mastersGOMAXPROCS1() {
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev) // 恢复，避免影响后续实验

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for char := 'a'; char < 'a'+26; char++ {
			fmt.Printf("%c ", char)
		}
	}()

	go func() {
		defer wg.Done()
		for number := 1; number < 27; number++ {
			fmt.Printf("%d ", number)
		}
	}()

	wg.Wait()
	fmt.Println("\n（GOMAXPROCS=1：两个 goroutine 不交错，哪个先跑完由调度决定，与启动顺序无关）")
}

// mastersGoschedSay 笔记 13 §4.1：Gosched 主动让出时间片，实现交替输出（文档注明 GOMAXPROCS=1 时）。
func mastersGoschedSay() {
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev) // 单核下 Gosched 的交替效果最明显

	say := func(s string, wg *sync.WaitGroup) {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			runtime.Gosched()
			fmt.Println(s)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go say("world", &wg)
	say("hello", &wg)
	wg.Wait()
	fmt.Println("（Gosched 让两个 goroutine 交替执行；多核下则可能各自连续输出，交替无保证）")
}

// mastersChannelSum 笔记 13 §4.1：两个 goroutine 各算一半，channel 汇总。
func mastersChannelSum() {
	sum := func(a []int, c chan int) {
		total := 0
		for _, v := range a {
			total += v
		}
		c <- total
	}

	a := []int{7, 2, 8, -9, 4, 0}
	c := make(chan int)
	go sum(a[:len(a)/2], c)
	go sum(a[len(a)/2:], c)
	x, y := <-c, <-c
	fmt.Printf("x=%d y=%d x+y=%d\n", x, y, x+y)
}

// mastersFibonacci 笔记 13 §4.1：生产者 close，range 自动退出。
func mastersFibonacci() {
	fibonacci := func(n int, c chan int) {
		x, y := 1, 1
		for i := 0; i < n; i++ {
			c <- x
			x, y = y, x+y
		}
		close(c) // 生产者关闭 channel
	}

	c := make(chan int, 10)
	go fibonacci(cap(c), c)
	for i := range c {
		fmt.Printf("%d ", i)
	}
	fmt.Println("（range 在 close 后自动退出）")
}

// mastersPipeline 笔记 13 §6.1：gen → sq → sq 三阶段流水线。
func mastersPipeline() {
	gen := func(nums ...int) <-chan int {
		out := make(chan int)
		go func() {
			for _, n := range nums {
				out <- n
			}
			close(out)
		}()
		return out
	}

	sq := func(in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			for n := range in {
				out <- n * n
			}
			close(out)
		}()
		return out
	}

	for n := range sq(sq(gen(2, 3))) {
		fmt.Println(n) // 16, 81
	}
}

// mastersSelectTimeout 笔记 13 §4.1：select + 超时退出模式。
func mastersSelectTimeout() {
	c := make(chan int)
	o := make(chan bool)

	go func() {
		for {
			select {
			case v := <-c:
				fmt.Println(v)
			case <-time.After(300 * time.Millisecond):
				fmt.Println("timeout")
				o <- true
				return
			}
		}
	}()

	<-o
	fmt.Println("（超时后主动退出，不泄漏 goroutine）")
}
