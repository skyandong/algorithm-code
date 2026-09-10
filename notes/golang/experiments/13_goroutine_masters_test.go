// 实验 13（名家并发模式汇总），断言验证
// 对应笔记：notes/golang/12-名家并发模式汇总.md
// 源码对照：golang.org/x/sync/errgroup
package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
