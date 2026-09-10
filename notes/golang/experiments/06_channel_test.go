// 实验 06（channel 与 nil 语义），断言验证
// 对应笔记：notes/golang/05-Channel内部与nil语义.md
// 源码对照：src/runtime/chan.go
package main

import (
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
		assert.Equal(t, 42, v)
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
	assert.Equal(t, 10, v)
	assert.True(t, ok)

	v, ok = <-ch
	assert.Equal(t, 20, v)
	assert.True(t, ok)

	v, ok = <-ch
	assert.Equal(t, 0, v)
	assert.False(t, ok, "关闭且排空后返回零值 + false")

	ch2 := make(chan int, 1)
	ch2 <- 0
	close(ch2)
	v, ok = <-ch2
	assert.Equal(t, 0, v)
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
		assert.Equal(t, 0, r.v)
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

	assert.Equal(t, 2, <-ch)
	assert.Equal(t, 3, <-ch)
}

// TestClosePanics 第 10 节：双重 close / 已关再发 / close(nil) 全 panic
func TestClosePanics(t *testing.T) {
	ch := make(chan int)
	close(ch)
	assert.Panics(t, func() { close(ch) }, "双重 close panic")

	ch2 := make(chan int)
	close(ch2)
	assert.Panics(t, func() { ch2 <- 1 }, "向已关闭 channel 发送：立即 panic，不阻塞")

	var nilCh chan int
	assert.Panics(t, func() { close(nilCh) }, "close(nil) panic")

	ch3 := make(chan int)
	close(ch3)
	v, ok := <-ch3
	assert.Equal(t, 0, v)
	assert.False(t, ok, "已关闭 channel 接收合法且永不阻塞——只有发送和 close 会炸")
}
