// 实验 12（Goroutine 面试题）的单元测试
//
// 纯逻辑部分：blockingMap / Ban 限流 / WaitTimeout 都是可以直接断言的可复用实现；
// 冒烟部分：完整跑一遍六道手写题。
package main

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestBlockingMapExisting 已存在的 key 立即返回。
func TestBlockingMapExisting(t *testing.T) {
	m := newBlockingMap()
	m.Out("k1", "v1")

	assert.Equal(t, "v1", m.Rd("k1", time.Second))
}

// TestBlockingMapWakeup 等待中的 Rd 被 Out 唤醒并拿到值。
func TestBlockingMapWakeup(t *testing.T) {
	m := newBlockingMap()

	type result struct {
		v  any
		at time.Time
	}
	done := make(chan result, 1)
	go func() {
		v := m.Rd("k2", 5*time.Second)
		done <- result{v: v, at: time.Now()}
	}()

	time.Sleep(50 * time.Millisecond)
	putAt := time.Now()
	m.Out("k2", "v2")

	got := <-done
	assert.Equal(t, "v2", got.v)
	assert.True(t, got.at.After(putAt), "Rd 应阻塞到 Out 之后才返回")
}

// TestBlockingMapTimeout 超时返回 nil，且不阻塞后续 Out。
func TestBlockingMapTimeout(t *testing.T) {
	m := newBlockingMap()

	assert.Nil(t, m.Rd("missing", 100*time.Millisecond))

	m.Out("missing", "late") // 超时的等待者应已被清理，不能 panic
	assert.Equal(t, "late", m.Rd("missing", time.Second))
}

// TestBanVisit 三分钟窗口内同 IP 只放行一次（注入时间，完全确定性）。
func TestBanVisit(t *testing.T) {
	ban := NewBan()
	t0 := time.Now()

	assert.False(t, ban.visit("1.2.3.4", t0), "首次访问放行")
	assert.True(t, ban.visit("1.2.3.4", t0.Add(time.Minute)), "窗口内再访被限")
	assert.True(t, ban.visit("1.2.3.4", t0.Add(2*time.Minute)), "窗口内再访被限")
	assert.False(t, ban.visit("1.2.3.4", t0.Add(3*time.Minute)), "窗口过期放行")
	assert.True(t, ban.visit("1.2.3.4", t0.Add(3*time.Minute+time.Second)), "重新计时")

	assert.False(t, ban.visit("5.6.7.8", t0), "不同 IP 互不影响")
}

// TestWaitTimeout 超时返回 true（调用方负责取消）、自然结束返回 false。
func TestWaitTimeout(t *testing.T) {
	// 场景 1：worker 不退出 → 超时 true
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-stop:
			case <-time.After(10 * time.Second):
			}
		}()
	}
	assert.True(t, WaitTimeout(&wg, 100*time.Millisecond))
	close(stop)
	wg.Wait()

	// 场景 2：任务快速完成 → false
	var wg2 sync.WaitGroup
	wg2.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg2.Done()
			time.Sleep(20 * time.Millisecond)
		}()
	}
	assert.False(t, WaitTimeout(&wg2, 2*time.Second))
}

// TestInterviewSmoke 全量冒烟。
func TestInterviewSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunInterviewExperiments)
	assert.Contains(t, out, "题3：高并发 IP 限流", "六道手写题必须全部出现")
	assert.Contains(t, out, "题6：多协程查询切片")
	assert.Contains(t, out, "Found it!", "context 取消查找必须演示")
	assert.Contains(t, out, "（0~9 各出现一次", "题19 range 闭包结论必须出现")
}
