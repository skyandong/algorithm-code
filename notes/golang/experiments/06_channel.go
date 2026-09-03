// # Channel 与 nil channel 语义实验
//
// 对应笔记：notes/golang/02-Channel内部与nil语义.md
//
// 运行：
//
//	go run ./experiments/ channel
//
// 实验项：
//
//	Exp1：nil channel 接收 case 永不就绪（select + timeout 兜底）
//	Exp2：nil channel 发送 case 配合 default 非阻塞
//	Exp3：用 nil 禁用 / 恢复 select 分支
//	Exp4：已关闭 channel 的接收语义 — 先读空缓冲，再返回零值 ok=false
//	Exp5：阻塞中的接收者被 close 唤醒（得到零值 + ok=false）
//	Exp6：阻塞中的发送者在 close 后 panic（recover 演示）
package main

import (
	"fmt"
	"sync"
	"time"
)

// RunChannelExperiments 演示笔记 2 的核心语义。
func RunChannelExperiments() {
	fmt.Println("===== 1. nil channel 接收 case 永不就绪（select + timeout）=====")
	demoNilRecvNeverReady()

	fmt.Println("\n===== 2. nil channel 发送 case 配合 default 非阻塞 =====")
	demoNilSendNonBlocking()

	fmt.Println("\n===== 3. 用 nil 禁用 / 恢复 select 分支 =====")
	demoDisableRestoreBranch()

	fmt.Println("\n===== 4. 已关闭 channel 的接收语义 =====")
	demoClosedRecv()

	fmt.Println("\n===== 5. 阻塞中的接收者被 close 唤醒 =====")
	demoCloseWakesReceiver()

	fmt.Println("\n===== 6. 阻塞中的发送者在 close 后 panic（recover 演示）=====")
	demoCloseWakesSenderPanic()
}

// demoNilRecvNeverReady 笔记 2 第 5.1 节：从 nil channel 接收永远不会完成。
func demoNilRecvNeverReady() {
	var ch chan int // nil

	select {
	case v := <-ch:
		fmt.Println("收到：", v)
	case <-time.After(300 * time.Millisecond):
		fmt.Println("timeout：nil channel 接收 case 永远不会就绪")
	}
}

// demoNilSendNonBlocking 笔记 2 第 5.3 节：nil channel 非阻塞发送立即失败。
func demoNilSendNonBlocking() {
	var ch chan int // nil

	select {
	case ch <- 1:
		fmt.Println("发送成功")
	default:
		fmt.Println("发送失败：nil channel 非阻塞发送立即失败，走 default")
	}
}

// demoDisableRestoreBranch 笔记 2 第 6 节：ch = nil 只是改变变量指向，可恢复。
func demoDisableRestoreBranch() {
	ch := make(chan int, 1)
	var input <-chan int = ch

	input = nil // 禁用分支
	select {
	case v := <-input:
		fmt.Println("收到：", v)
	default:
		fmt.Println("input 已禁用（ch=nil 只是变量指向 nil，channel 未被关闭）")
	}

	input = ch // 恢复分支
	ch <- 42
	select {
	case v := <-input:
		fmt.Println("恢复后收到：", v)
	default:
		fmt.Println("没有数据")
	}
}

// demoClosedRecv 笔记 2 第 11 节：close 不清空缓冲，读完数据后才返回零值 + false。
func demoClosedRecv() {
	ch := make(chan int, 2)
	ch <- 10
	ch <- 20
	close(ch) // 关闭发送入口，保留缓冲数据

	v, ok := <-ch
	fmt.Printf("第一次接收：%d, ok=%v\n", v, ok) // 10 true
	v, ok = <-ch
	fmt.Printf("第二次接收：%d, ok=%v\n", v, ok) // 20 true
	v, ok = <-ch
	fmt.Printf("第三次接收：%d, ok=%v（已关闭且排空，返回零值）\n", v, ok) // 0 false

	// 注意：不能只靠值判断关闭 —— 发送方也可能发零值
	ch2 := make(chan int, 1)
	ch2 <- 0
	close(ch2)
	v, ok = <-ch2
	fmt.Printf("收到零值：%d, ok=%v（真实发送的零值，ok 为 true）\n", v, ok)
}

// demoCloseWakesReceiver 笔记 2 第 12.1 节：阻塞接收者被 close 唤醒，得到零值 + false。
func demoCloseWakesReceiver() {
	ch := make(chan int)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, ok := <-ch
		fmt.Printf("被唤醒的接收者：v=%d, ok=%v\n", v, ok)
	}()

	time.Sleep(100 * time.Millisecond)
	close(ch) // 唤醒接收者，标记失败
	wg.Wait()
}

// demoCloseWakesSenderPanic 笔记 2 第 12.2 节：阻塞发送者被 close 唤醒后 panic。
func demoCloseWakesSenderPanic() {
	ch := make(chan int, 1)
	ch <- 1 // 缓冲区已满

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("发送者 recover：", r, "（close 不会让阻塞发送成功，而是 panic）")
			}
		}()
		ch <- 2 // 阻塞在 sendq
	}()

	time.Sleep(100 * time.Millisecond)
	close(ch)
	wg.Wait()
}
