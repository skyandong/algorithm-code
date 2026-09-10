// 实验 13（名家并发模式汇总），断言验证
// 对应笔记：notes/golang/12-名家并发模式汇总.md
// 源码对照：golang.org/x/sync/errgroup
package main

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ===== 实验实现（原 13_goroutine_masters.go，已并入本文件）=====

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

// ===== 断言用例 =====

// TestBroadcastStop close 一次广播，100 个 goroutine 在 2s 内全部收到并退出。
func TestBroadcastStop(t *testing.T) {
	done := make(chan struct{})
	go func() {
		broadcastStop(100)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("close 广播未能在 2s 内唤醒 100 个 goroutine")
	}
}

// TestWaitMany nil channel 禁用已关闭分支，消费完两个 channel 后正常退出。
func TestWaitMany(t *testing.T) {
	a, b := make(chan bool), make(chan bool)
	go func() {
		close(a)
		close(b)
	}()

	done := make(chan struct{})
	go func() {
		waitMany(a, b)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("nil channel 未禁用已关闭分支，陷入死循环")
	}
}

// TestParallelSum 两个 goroutine 各算一半，channel 汇总与顺序求和一致。
func TestParallelSum(t *testing.T) {
	assert.Equal(t, 12, parallelSum([]int{7, 2, 8, -9, 4, 0}))
}

// TestFibonacci 生产者 close 后 range 自动退出，产出前 7 个斐波那契数。
func TestFibonacci(t *testing.T) {
	assert.Equal(t, []int{1, 1, 2, 3, 5, 8, 13}, fibonacci(7))
}

// TestPipeline gen→sq→sq：2→4→16，3→9→81。
func TestPipeline(t *testing.T) {
	assert.Equal(t, []int{16, 81}, pipeline(2, 3))
}

// TestSelectWithTimeout 无发送方超时返回 true；有值及时返回 false。
func TestSelectWithTimeout(t *testing.T) {
	c := make(chan int)
	v, timedOut := selectWithTimeout(c, 100*time.Millisecond)
	assert.True(t, timedOut, "无发送方应在超时分支退出")
	assert.Equal(t, 0, v)

	ready := make(chan int, 1)
	ready <- 42
	v, timedOut = selectWithTimeout(ready, 100*time.Millisecond)
	assert.False(t, timedOut, "有值应立即返回，不进超时分支")
	assert.Equal(t, 42, v)
}
