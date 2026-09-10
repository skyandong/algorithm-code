// 实验 05（并发内存可见性），断言验证
// 对应笔记：notes/golang/04-并发内存可见性与sync.Once.md
// 源码对照：src/sync/once.go、src/sync/mutex.go
// 竞态检测：go test -race ./experiments/（无同步的 race 演示见笔记第 1 节，不进常规用例）
package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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

func TestAtomicCounterKeepsAllUpdates(t *testing.T) {
	var counter atomic.Int64
	var wg sync.WaitGroup

	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				counter.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(8000), counter.Load(), "atomic 不丢更新")
}

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

	assert.True(t, ready)
	assert.Equal(t, 42, data, "Unlock happens-before 之后的 Lock")
}

func TestOnceRunsExactlyOnce(t *testing.T) {
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

	assert.Equal(t, int64(1), executed.Load(), "sync.Once 只执行一次，完成后对所有人可见")
}
