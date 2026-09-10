// 实验 12（Goroutine 面试题集），可复用实现（断言见 12_goroutine_interview_test.go）
// 对应笔记：notes/golang/11-Goroutine面试题集.md
// 源码对照：src/runtime/runtime2.go（阻塞在 channel 上的 G 的等待凭证 sudog）
//
//	go test -run 'TestBlockingMap|TestBan|TestWaitTimeout|TestFind|TestRangeClosure|TestProducerConsumer|TestTimerPanic' ./experiments/
package main

import (
	"context"
	"sync"
	"time"
)

// waiter 阻塞读 Map 的等待者凭证。
type waiter struct {
	done chan struct{}
}

// blockingMap 支持阻塞读的并发安全 Map：Out 关闭 channel 唤醒所有等待者，Rd 支持超时。
type blockingMap struct {
	mu      sync.Mutex
	values  map[string]any
	waiters map[string]map[*waiter]struct{}
}

func newBlockingMap() *blockingMap {
	return &blockingMap{
		values:  make(map[string]any),
		waiters: make(map[string]map[*waiter]struct{}),
	}
}

func (m *blockingMap) Out(key string, value any) {
	m.mu.Lock()
	m.values[key] = value
	for w := range m.waiters[key] {
		close(w.done) // 关闭 channel 唤醒所有等待者
	}
	delete(m.waiters, key)
	m.mu.Unlock()
}

func (m *blockingMap) Rd(key string, timeout time.Duration) any {
	m.mu.Lock()
	if value, ok := m.values[key]; ok {
		m.mu.Unlock()
		return value
	}

	w := &waiter{done: make(chan struct{})}
	if m.waiters[key] == nil {
		m.waiters[key] = make(map[*waiter]struct{})
	}
	m.waiters[key][w] = struct{}{}
	m.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-w.done:
		m.mu.Lock()
		value := m.values[key]
		m.mu.Unlock()
		return value
	case <-timer.C:
		m.mu.Lock()
		if _, waiting := m.waiters[key][w]; waiting {
			delete(m.waiters[key], w)
			if len(m.waiters[key]) == 0 {
				delete(m.waiters, key)
			}
			m.mu.Unlock()
			return nil
		}
		value := m.values[key]
		m.mu.Unlock()
		return value
	}
}

// Ban 高并发 IP 限流：三分钟窗口内同 IP 只放行一次（注入时间，完全确定性）。
type Ban struct {
	mu       sync.Mutex
	visitIPs map[string]time.Time
}

func NewBan() *Ban {
	return &Ban{visitIPs: make(map[string]time.Time)}
}

func (b *Ban) visit(ip string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if last, ok := b.visitIPs[ip]; ok && now.Sub(last) < 3*time.Minute {
		return true
	}
	b.visitIPs[ip] = now
	return false
}

// WaitTimeout 给 WaitGroup 加超时：超时返回 true（调用方负责取消），自然结束返回 false。
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return false
	case <-timer.C:
		return true
	}
}

// findWithTimeout 多协程查找目标值，找到即取消其余 worker，返回 "found"/"notfound"/"timeout"。
func findWithTimeout(values []int, target, workers int, timeout time.Duration) string {
	return searchValues(values, target, workers, timeout, 0)
}

// findSlowWithTimeout 同 findWithTimeout，但每元素检查前加微小延时，确定性触发超时取消路径。
func findSlowWithTimeout(values []int, target, workers int, timeout time.Duration) string {
	return searchValues(values, target, workers, timeout, 2*time.Microsecond)
}

func searchValues(values []int, target, workers int, timeout, perElem time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	found := make(chan struct{}, 1)
	var wg sync.WaitGroup

	if workers > len(values) {
		workers = len(values)
	}
	if workers == 0 {
		return "timeout"
	}

	for i := 0; i < workers; i++ {
		start := len(values) * i / workers
		end := len(values) * (i + 1) / workers
		part := values[start:end]

		wg.Go(func() {
			for _, value := range part {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if perElem > 0 {
					time.Sleep(perElem) // 模拟慢查询，确定性拉长扫描
				}

				if value == target {
					select {
					case found <- struct{}{}:
						cancel()
					default:
					}
					return
				}
			}
		})
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	select {
	case <-found:
		cancel()
		<-finished
		return "found"
	case <-finished:
		select {
		case <-found:
			return "found"
		default:
			if ctx.Err() == context.DeadlineExceeded {
				return "timeout"
			}
			return "notfound"
		}
	}
}
