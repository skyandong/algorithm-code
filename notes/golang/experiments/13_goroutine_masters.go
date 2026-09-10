// 实验 13（名家并发模式汇总），可复用实现（断言见 13_goroutine_masters_test.go）
// 对应笔记：notes/golang/12-名家并发模式汇总.md
// 源码对照：golang.org/x/sync/errgroup（errOnce 只记首个错误，任一任务出错即 cancel 派生 ctx）
//
//	go test -run 'TestBroadcastStop|TestWaitMany|TestParallelSum|TestFibonacci|TestPipeline|TestSelectWithTimeout' ./experiments/
package main

import (
	"sync"
	"time"
)

// broadcastStop close(finish) 一次广播，n 个 goroutine 同时收到并退出。
func broadcastStop(n int) {
	finish := make(chan struct{})
	var done sync.WaitGroup

	done.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer done.Done()
			select {
			case <-time.After(time.Hour):
			case <-finish:
			}
		}()
	}

	close(finish)
	done.Wait()
}

// waitMany nil channel 禁用已关闭分支，两个 channel 都消费完即正常退出。
func waitMany(a, b chan bool) {
	for a != nil || b != nil {
		select {
		case <-a:
			a = nil
		case <-b:
			b = nil
		}
	}
}

// parallelSum 两 goroutine 各算一半，channel 汇总返回总和。
func parallelSum(a []int) int {
	c := make(chan int, 2)
	sum := func(part []int) {
		total := 0
		for _, v := range part {
			total += v
		}
		c <- total
	}
	go sum(a[:len(a)/2])
	go sum(a[len(a)/2:])
	return <-c + <-c
}

// fibonacci 生产者 close channel，消费者 range 自动退出，返回前 n 个斐波那契数。
func fibonacci(n int) []int {
	c := make(chan int, n)
	go func() {
		x, y := 1, 1
		for i := 0; i < n; i++ {
			c <- x
			x, y = y, x+y
		}
		close(c)
	}()

	out := make([]int, 0, n)
	for v := range c {
		out = append(out, v)
	}
	return out
}

// pipeline gen → sq → sq 三阶段流水线，返回最终平方的平方序列。
func pipeline(nums ...int) []int {
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

	out := make([]int, 0)
	for n := range sq(sq(gen(nums...))) {
		out = append(out, n)
	}
	return out
}

// selectWithTimeout 读 c 或超时二选一；返回值与是否超时。
func selectWithTimeout(c <-chan int, timeout time.Duration) (int, bool) {
	select {
	case v := <-c:
		return v, false
	case <-time.After(timeout):
		return 0, true
	}
}
