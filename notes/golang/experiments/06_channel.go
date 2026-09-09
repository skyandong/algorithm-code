// # Channel 与 nil channel 语义实验
//
// 对应笔记：notes/golang/05-Channel内部与nil语义.md
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
//	Exp7：无缓冲 channel 直接交接（发送阻塞到接收者就绪）
//	Exp8：缓冲满时发送阻塞（sendq 挂起），腾位后立即成功
//	Exp9：close 补充语义 — 双重 close / 已关再发 / close(nil) 全 panic
//	Exp10：nil/活跃/已关闭 × 发送/接收/close 状态转换表
//
// —— runtime 源码对照 ——
//
// src/runtime/chan.go（节选，省略 elemsize/elemtype/timer/bubble）
//
//	type hchan struct {
//		qcount   uint           // 当前队列元素数
//		dataqsiz uint           // 环形队列容量（make 的第二个参数）
//		buf      unsafe.Pointer // 环形队列数据区
//		sendx    uint           // 发送索引
//		recvx    uint           // 接收索引
//		closed   uint32         // 关闭标志
//		recvq    waitq          // 被 <-ch 阻塞的接收者队列（sudog 链表）
//		sendq    waitq          // 被 ch<- 阻塞的发送者队列
//		lock     mutex          // 保护以上全部字段（含阻塞在这上面的 sudog 部分字段）
//	}
package main

import (
	"fmt"
	"sync"
	"time"
)

// RunChannelExperiments 演示笔记 6 的核心语义。
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

	fmt.Println("\n===== 7. 无缓冲 channel：发送者阻塞到接收者就绪（直接交接）=====")
	demoUnbufferedHandoff()

	fmt.Println("\n===== 8. 缓冲满时发送阻塞（环形队列无空位）=====")
	demoBufferedFullBlock()

	fmt.Println("\n===== 9. close 的补充语义：双重 close / 已关再发 / close nil =====")
	demoCloseSemantics()

	fmt.Println("\n===== 10. channel 状态转换表 =====")
	demoStateTable()
}

// demoNilRecvNeverReady 笔记 6 第 5.1 节：从 nil channel 接收永远不会完成。
func demoNilRecvNeverReady() {
	var ch chan int // nil

	select {
	case v := <-ch:
		fmt.Println("收到：", v)
	case <-time.After(300 * time.Millisecond):
		fmt.Println("timeout：nil channel 接收 case 永远不会就绪")
	}
}

// demoNilSendNonBlocking 笔记 6 第 5.3 节：nil channel 非阻塞发送立即失败。
func demoNilSendNonBlocking() {
	var ch chan int // nil

	select {
	case ch <- 1:
		fmt.Println("发送成功")
	default:
		fmt.Println("发送失败：nil channel 非阻塞发送立即失败，走 default")
	}
}

// demoDisableRestoreBranch 笔记 6 第 6 节：ch = nil 只是改变变量指向，可恢复。
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

// demoClosedRecv 笔记 6 第 11 节：close 不清空缓冲，读完数据后才返回零值 + false。
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

// demoCloseWakesReceiver 笔记 6 第 12.1 节：阻塞接收者被 close 唤醒，得到零值 + false。
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

// demoCloseWakesSenderPanic 笔记 6 第 12.2 节：阻塞发送者被 close 唤醒后 panic。
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

// demoUnbufferedHandoff 笔记 06 第 8 节：无缓冲发送阻塞到接收者就绪，数据直接从 S->R 交接。
func demoUnbufferedHandoff() {
	ch := make(chan int)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond) // 接收者 200ms 后才来
		start := time.Now()
		v := <-ch
		fmt.Printf("接收者 %v 后收到 %d（数据直接从发送栈拷到接收栈，不经过缓冲）\n",
			time.Since(start).Round(10*time.Millisecond), v)
	}()

	start := time.Now()
	ch <- 42
	fmt.Printf("发送返回耗时 %v（发送者一直阻塞到接收者就绪才完成交接）\n",
		time.Since(start).Round(10*time.Millisecond))
	wg.Wait()
}

// demoBufferedFullBlock 笔记 06 第 9 节：缓冲满时发送者进 sendq 阻塞。
func demoBufferedFullBlock() {
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2 // 环形队列已满

	select {
	case ch <- 3:
		fmt.Println("发送成功（不应出现）")
	case <-time.After(200 * time.Millisecond):
		fmt.Println("缓冲满：发送阻塞（sendq 挂起），timeout 兜底证明不会写入")
	}

	<-ch // 腾出一个空位
	ch <- 3
	fmt.Println("腾出空位后 ch <- 3 立即成功（有空位 = 直接写环形队列，不阻塞）")
}

// demoCloseSemantics 笔记 06 第 10 节：close 的三条红线 —— 双重 close、已关再发、close nil。
func demoCloseSemantics() {
	// 1) 双重 close：panic
	func() {
		defer func() { fmt.Println("双重 close → panic:", recover()) }()
		ch := make(chan int)
		close(ch)
		close(ch)
	}()

	// 2) 向已关闭 channel 发送：不阻塞，立即 panic（nil channel 发送才阻塞）
	func() {
		defer func() { fmt.Println("已关闭再发送 → panic:", recover()) }()
		ch := make(chan int)
		close(ch)
		ch <- 1
	}()

	// 3) close(nil channel)：panic（close 只认有 hchan 的活跃 channel）
	func() {
		defer func() { fmt.Println("close(nil) → panic:", recover()) }()
		var ch chan int
		close(ch)
	}()

	// 4) 已关闭的 channel 可以继续接收（读完缓冲返回零值，见第 11 节）——只有发送和 close 会炸
	ch := make(chan int)
	close(ch)
	v, ok := <-ch
	fmt.Printf("已关闭的 channel 接收: v=%d ok=%v（合法且永不阻塞）\n", v, ok)
}

// demoStateTable 笔记 06 第 13/14/15 节：nil/活跃/已关闭 × 发送/接收/关闭 的完整行为表。
func demoStateTable() {
	fmt.Println("状态       | 发送 send        | 接收 recv              | close")
	fmt.Println("-----------+------------------+------------------------+-----------")
	fmt.Println("nil        | 永久阻塞         | 永久阻塞               | panic")
	fmt.Println("活跃(非nil) | 满则阻塞否则成功 | 空则阻塞否则成功       | 正常")
	fmt.Println("已关闭     | panic            | 零值+false，永不阻塞   | panic(重复关)")
	fmt.Println()
	fmt.Println("区分 deadlock 与 nil 阻塞: nil channel 阻塞不报 deadlock（runtime 认为在等一个永不")
	fmt.Println("就绪的 case）；select 全 nil 分支 + 无 default 才是 deadlock 检测的目标")
	fmt.Println("nil 禁用 vs close 关闭（第 15 节）: ch=nil 只是变量指向，可随时恢复；close 是协议动作，不可逆")
}
