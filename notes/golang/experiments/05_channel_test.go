// 实验 05（channel 与 nil 语义），断言验证
// 对应笔记：notes/golang/05-channel内部与nil语义.md
// 源码对照：src/runtime/chan.go
package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNilChannelNeverReady 第 5 节：nil channel 收发永不就绪
func TestNilChannelNeverReady(t *testing.T) {
	var ch chan int // nil

	select {
	case v := <-ch:
		t.Fatalf("nil channel 接收不应就绪，却收到 %d", v)
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case ch <- 1:
		t.Fatal("nil channel 发送不应就绪")
	default:
	}
}

// TestNilDisablesSelectBranch 第 6 节：ch=nil 只是变量指向，可随时恢复
func TestNilDisablesSelectBranch(t *testing.T) {
	ch := make(chan int, 1)
	var input <-chan int = ch

	input = nil // 禁用分支：只是变量指向 nil，channel 没被关闭
	select {
	case v := <-input:
		t.Fatalf("input=nil 分支应失效，却收到 %d", v)
	default:
	}

	input = ch // 恢复分支
	ch <- 42
	select {
	case v := <-input:
		assert.Equal(t, 42, v, "恢复指向后分支重新就绪取到 42")
	default:
		t.Fatal("恢复指向后分支应就绪")
	}
}

// TestClosedChannelReceive 第 11 节：关闭后先排空缓冲，再返回零值 + false
func TestClosedChannelReceive(t *testing.T) {
	ch := make(chan int, 2)
	ch <- 10
	ch <- 20
	close(ch) // 关闭发送入口，缓冲数据保留

	v, ok := <-ch
	assert.Equal(t, 10, v, "关闭后先排空缓冲，先收到 10")
	assert.True(t, ok, "缓冲内数据 ok 仍为 true")

	v, ok = <-ch
	assert.Equal(t, 20, v, "关闭后排空第二项 20")
	assert.True(t, ok, "缓冲内第二项 ok 仍为 true")

	v, ok = <-ch
	assert.Equal(t, 0, v, "关闭且排空后收到零值")
	assert.False(t, ok, "关闭且排空后返回零值 + false")

	ch2 := make(chan int, 1)
	ch2 <- 0
	close(ch2)
	v, ok = <-ch2
	assert.Equal(t, 0, v, "真实发送的零值仍被取出")
	assert.True(t, ok, "真实发送的零值 ok 仍为 true——不能只靠值判断关闭")
}

// TestCloseWakesReceiver 第 12 节：close 唤醒阻塞的接收者
func TestCloseWakesReceiver(t *testing.T) {
	ch := make(chan int)

	type res struct {
		v  int
		ok bool
	}
	got := make(chan res)
	go func() {
		v, ok := <-ch
		got <- res{v, ok}
	}()

	time.Sleep(50 * time.Millisecond)
	close(ch)

	select {
	case r := <-got:
		assert.Equal(t, 0, r.v, "被 close 唤醒的接收者拿到零值")
		assert.False(t, r.ok, "被 close 唤醒的接收者拿到零值 + false")
	case <-time.After(time.Second):
		t.Fatal("close 应唤醒阻塞中的接收者")
	}
}

// TestCloseWakesSenderWithPanic 第 12 节：close 唤醒阻塞的发送者，后者 panic
func TestCloseWakesSenderWithPanic(t *testing.T) {
	if raceEnabled {
		t.Skip("-race 下 closechan 唤醒阻塞 sender 的路径会被 race detector 保守报告（最小重现同样复现）")
	}

	ch := make(chan int, 1)
	ch <- 1 // 缓冲已满

	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		ch <- 2 // 阻塞在 sendq
	}()

	time.Sleep(50 * time.Millisecond)
	close(ch)

	select {
	case r := <-done:
		assert.NotNil(t, r, "close 唤醒阻塞发送者 → panic（不是发送成功）")
	case <-time.After(time.Second):
		t.Fatal("发送者应被 close 唤醒")
	}
}

// TestUnbufferedHandoffBlocksSender 第 8 节：无缓冲直接交接，发送阻塞到接收就绪
func TestUnbufferedHandoffBlocksSender(t *testing.T) {
	ch := make(chan int)

	go func() {
		time.Sleep(100 * time.Millisecond) // 接收者 100ms 后才来
		<-ch
	}()

	start := time.Now()
	ch <- 42
	assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond,
		"无缓冲：发送者一直阻塞到接收者就绪才交接")
}

// TestBufferedFullBlocks 第 9 节：缓冲满则发送阻塞，腾位后立即成功
func TestBufferedFullBlocks(t *testing.T) {
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2 // 环形队列已满

	select {
	case ch <- 3:
		t.Fatal("缓冲已满，发送不应成功")
	case <-time.After(50 * time.Millisecond):
	}

	<-ch // 腾出一个空位
	select {
	case ch <- 3:
	default:
		t.Fatal("有空位后发送应立即成功")
	}

	assert.Equal(t, 2, <-ch, "腾位后缓冲 FIFO 先出旧的 2")
	assert.Equal(t, 3, <-ch, "新发的 3 排在队尾随后出队")
}

// TestClosePanics 第 10 节：双重 close / 已关再发 / close(nil) 全 panic
// （三条红线只涉及发送与 close；接收永远合法，见 TestClosedChannelReceive）
func TestClosePanics(t *testing.T) {
	ch := make(chan int)
	close(ch)
	assert.Panics(t, func() { close(ch) }, "双重 close panic")

	ch2 := make(chan int)
	close(ch2)
	assert.Panics(t, func() { ch2 <- 1 }, "向已关闭 channel 发送：立即 panic，不阻塞")

	var nilCh chan int
	assert.Panics(t, func() { close(nilCh) }, "close(nil) panic")
}

// TestCloseBroadcastAllWaiters 第 12.1 节：close 是广播——一次关闭唤醒 recvq 里的全部 100 个等待者
// TestCloseWakesReceiver 只放了一个等待者，这条验的是「逐个排空唤醒」而不是「只唤醒一个」
func TestCloseBroadcastAllWaiters(t *testing.T) {
	const waiters = 100

	ch := make(chan struct{})
	var woke sync.WaitGroup
	woke.Add(waiters)
	for i := 0; i < waiters; i++ {
		go func() {
			defer woke.Done()
			<-ch // 全部 park 进 recvq
		}()
	}

	time.Sleep(50 * time.Millisecond) // 等所有等待者真的 park 完
	close(ch)                         // 一次 close = 一次广播

	done := make(chan struct{})
	go func() { woke.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("close 应唤醒 recvq 里的全部等待者：只唤醒一个则其余永久阻塞")
	}
}

// TestDrainMultiInputThenNil 第 6.5 节：已关闭且排空的分支必须置 nil，否则 select 反复命中它空转
func TestDrainMultiInputThenNil(t *testing.T) {
	ch1 := make(chan int, 1)
	ch1 <- 1
	close(ch1) // 有缓冲值，排空后才进 false 分支

	ch2 := make(chan int)
	close(ch2) // 无缓冲且已关闭：每次读都立即返回 (0, false)

	var got []int
	for iter := 0; ch1 != nil || ch2 != nil; iter++ {
		if iter > 100 { // 保险：不置 nil 会在这里红，而不是把 go test 挂到超时
			t.Fatal("已关闭且排空的分支没置 nil，select 一直在命中它空转")
		}
		select {
		case v, ok := <-ch1:
			if !ok {
				ch1 = nil // 排空后置 nil：把该分支移出 select
				continue
			}
			got = append(got, v)
		case v, ok := <-ch2:
			if !ok {
				ch2 = nil
				continue
			}
			got = append(got, v)
		}
	}

	assert.Equal(t, []int{1}, got, "真值只取出一次，两个分支都置 nil 后靠循环条件退出——不置就是空转")
}

// TestRangeExitsOnClose 第 11 节：生产者 close 后 range 收口，缓冲内元素不重不漏、不追读零值
func TestRangeExitsOnClose(t *testing.T) {
	ch := make(chan int, 3)
	go func() {
		for _, v := range []int{1, 1, 2} {
			ch <- v
		}
		close(ch)
	}()

	var got []int
	for v := range ch {
		got = append(got, v)
	}

	assert.Equal(t, []int{1, 1, 2}, got, "range 在 close 后退出：缓冲内 3 个元素不多不少，也不多读一个零值")
}

// TestChannelCarriesResultNoLock 第 7、8 节：结果靠 channel 传递——两个 G 各写一次、主 G 读两次即可，无需加锁
func TestChannelCarriesResultNoLock(t *testing.T) {
	in := []int{7, 2, 8, -9, 4, 0}
	c := make(chan int, 2)

	sum := func(part []int) {
		total := 0
		for _, v := range part {
			total += v
		}
		c <- total // 结果通过 channel 传递，不写共享变量
	}
	go sum(in[:len(in)/2])
	go sum(in[len(in)/2:])

	want := 0
	for _, v := range in {
		want += v
	}
	assert.Equal(t, want, <-c+<-c, "两个 G 各写一次 channel、主 G 读两次汇总：传递结果不靠共享内存，也就没有竞态")
}

// ===== 实验实现：阻塞读的并发安全 Map（第 12 节）=====

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

// newBlockingMap 构造「带超时读取的并发安全 map」
func newBlockingMap() *blockingMap {
	return &blockingMap{
		values:  make(map[string]any),
		waiters: make(map[string]map[*waiter]struct{}),
	}
}

// Out 写入并唤醒等待这个 key 的读者
func (m *blockingMap) Out(key string, value any) {
	m.mu.Lock()
	m.values[key] = value
	for w := range m.waiters[key] {
		close(w.done) // 关闭 channel 唤醒所有等待者
	}
	delete(m.waiters, key)
	m.mu.Unlock()
}

// Rd 读取：有值直接返回，否则等到超时返回 nil
func (m *blockingMap) Rd(key string, timeout time.Duration) any {
	context.Background()
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

// ===== 断言用例 =====

// TestBlockingMapExisting 第 12 节：已存在的 key 立即返回。
func TestBlockingMapExisting(t *testing.T) {
	m := newBlockingMap()
	m.Out("k1", "v1")

	assert.Equal(t, "v1", m.Rd("k1", time.Second), "已存在的 key 立即返回存过的值")
}

// TestBlockingMapWakeup 第 12 节：等待中的 Rd 被 Out 唤醒并拿到值。
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
	assert.Equal(t, "v2", got.v, "被 Out 唤醒的 Rd 拿到写入值")
	assert.True(t, got.at.After(putAt), "Rd 应阻塞到 Out 之后才返回")
}

// TestBlockingMapTimeout 第 12 节：超时返回 nil，且不阻塞后续 Out。
func TestBlockingMapTimeout(t *testing.T) {
	m := newBlockingMap()

	assert.Nil(t, m.Rd("missing", 100*time.Millisecond), "缺失的 key 超时返回 nil")

	m.Out("missing", "late") // 超时的等待者应已被清理，不能 panic
	assert.Equal(t, "late", m.Rd("missing", time.Second), "超时清理后写入可被正常读回")
}
