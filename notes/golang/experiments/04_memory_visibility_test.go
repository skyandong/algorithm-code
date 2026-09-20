// 实验 04（并发内存可见性），断言验证
// 对应笔记：notes/golang/04-并发内存模型与可见性.md
// 源码对照：src/sync/mutex.go
// 竞态检测：go test -race ./experiments/（无同步的 race 演示见笔记第 1、3 节，不进常规用例）
package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestAtomicPublish 第 2 节：atomic 的内存顺序建立 happens-before
func TestAtomicPublish(t *testing.T) {
	var data atomic.Int64
	var ready atomic.Bool

	go func() {
		data.Store(42)
		ready.Store(true)
	}()

	deadline := time.Now().Add(time.Second)
	for !ready.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
	}

	assert.True(t, ready.Load(), "atomic 发布：ready 置位后必然可见")
	assert.Equal(t, int64(42), data.Load(), "顺序一致：见到 ready 就能见到 data")
}

// TestChannelPublish 第 2 节：close 通知建立 happens-before
func TestChannelPublish(t *testing.T) {
	done := make(chan struct{})
	var data int

	go func() {
		data = 42
		close(done)
	}()

	<-done
	assert.Equal(t, 42, data, "close 前的写入 happens-before 收到通知")
}

// TestMutexPublish 第 4 节：mutex 同时保证互斥与可见性
func TestMutexPublish(t *testing.T) {
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

	wg.Wait()
	mu.Lock()
	defer mu.Unlock()

	assert.True(t, ready, "Unlock 前的写入对后续 Lock 者可见")
	assert.Equal(t, 42, data, "Unlock happens-before 之后的 Lock")
}
