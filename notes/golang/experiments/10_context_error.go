// # Context 与错误处理实验
//
// 对应笔记：notes/golang/10-context与错误处理.md
//
// 运行（接入 main.go 后）：
//
//	go run ./experiments/ context
//
// 实验项：
//
//	第1节：取消沿树广播（父取消 → 全部子 Done 关闭）+ 子超时只能更紧
//	第2节：Done 是 channel 关闭（多监听者同时解除阻塞）+ ctx.Err()
//	第3节：WithTimeout 不 cancel 的泄漏观察 + WithoutCancel 剥离取消
//	第4节：errors.Is/As 沿错误树判定 + Join 多错误
//	第5节：panic/recover 的位置限制 + 跨 goroutine 无效
//	第6节：defer 参数立即求值 + 命名返回值改写
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// RunContextExperiments 演示笔记 10 的 context 与错误处理行为。
func RunContextExperiments() {
	fmt.Println("========== 第1节: 取消树广播与超时收紧 ==========")
	ctxCancelTree()

	fmt.Println("\n========== 第2节: Done 关闭的广播语义 ==========")
	ctxDoneBroadcast()

	fmt.Println("\n========== 第3节: 泄漏观察与 WithoutCancel ==========")
	ctxLeak()

	fmt.Println("\n========== 第4节: 错误链 Is/As/Join ==========")
	errChain()

	fmt.Println("\n========== 第5节: panic 与 recover 的边界 ==========")
	panicRecover()

	fmt.Println("\n========== 第6节: defer 求值时机 ==========")
	deferTraps()
}

// ctxCancelTree 第1节：父取消沿树传播；子超时不能比父宽。
func ctxCancelTree() {
	parent, cancel := context.WithCancel(context.Background())
	childA, cancelA := context.WithCancel(parent)            // 手动取消层
	childB, cancelB := context.WithTimeout(parent, time.Hour) // 子给了 1 小时
	defer cancelA()
	defer cancelB()
	defer cancel()

	var done sync.WaitGroup
	done.Add(2)
	go func() {
		defer done.Done()
		<-childA.Done()
		fmt.Println("childA Done")
	}()
	go func() {
		defer done.Done()
		<-childB.Done()
		fmt.Println("childB Done")
	}()

	cancel() // 父取消 → childA/childB 同一时刻关闭
	done.Wait()

	fmt.Println("父 cancel → 两个子 ctx 的 Done 全部关闭（广播，且子取消不影响父）")

	// 子超时只能更紧：父 100ms，子给 10s，实际 100ms 截止
	p, pcancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer pcancel()
	c, ccancel := context.WithTimeout(p, 10*time.Second)
	defer ccancel()
	start := time.Now()
	<-c.Done()
	fmt.Printf("父100ms子10s: 子实际 %v 后取消（Deadline 取更早者），Err=%v\n",
		time.Since(start).Round(10*time.Millisecond), c.Err())
}

// ctxDoneBroadcast 第2节：关闭 channel = 任意多监听者同时解除阻塞。
func ctxDoneBroadcast() {
	ctx, cancel := context.WithCancel(context.Background())

	const watchers = 3
	var wg sync.WaitGroup
	wg.Add(watchers)
	for i := 1; i <= watchers; i++ {
		go func(id int) {
			defer wg.Done()
			<-ctx.Done() // 三个监听者都会被唤醒——发送只能唤醒一个
		}(i)
	}

	time.Sleep(50 * time.Millisecond)
	cancel() // 关闭 done channel，广播
	wg.Wait()

	fmt.Println("3 个监听 <-ctx.Done() 全部解除阻塞（关闭是广播；发送会丢）")
	fmt.Printf("取消后 ctx.Err() = %v（另一个取值: context.DeadlineExceeded）\n", ctx.Err())
}

// ctxLeak 第3节：不 cancel 的 WithTimeout 在父长期存活时滞留子树。
func ctxLeak() {
	root, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// 对照组：正确版 cancel 立即执行（子 ctx 从 root 摘除，可回收）
	for i := 0; i < 1000; i++ {
		_, cancel := context.WithTimeout(root, time.Hour)
		cancel() // 正确姿势是 defer cancel，这里立即执行便于对比
	}
	leakedCancels := make([]context.CancelFunc, 0, 1000)
	for i := 0; i < 1000; i++ {
		_, lcancel := context.WithTimeout(root, time.Hour)
		leakedCancels = append(leakedCancels, lcancel) // ✗ 事故代码：cancel 被遗忘，子 ctx 挂在 root 上
	}

	// 观察子树规模：等一会让挂靠稳定
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("派生 2000 个子 ctx：1000 个已 cancel（摘除），%d 个未 cancel 挂在 root 上\n", len(leakedCancels))
	fmt.Println("后果: root 活多久它们活多久（timer + children 引用）——goroutine/RSS 缓慢上涨的经典来源")
	rootCancel() // 收尾：把泄漏的一并取消（实验环境清理）

	// WithoutCancel：继承 Value、剥离取消（Go 1.21+）
	type ctxKey struct{}
	parent, pcancel := context.WithTimeout(context.WithValue(context.Background(), ctxKey{}, "trace-42"), 50*time.Millisecond)
	defer pcancel()
	time.Sleep(80 * time.Millisecond) // 让父超时

	child := context.WithoutCancel(parent)
	fmt.Printf("父已超时, WithoutCancel 子: Done()==nil（永不可取消，同 Background）=%v, Value=%v（收尾场景：要 traceID 不要连坐取消）\n",
		child.Done() == nil, child.Value(ctxKey{}))
}

// 哨兵错误与结构化错误类型
var errConnRefused = errors.New("connection refused")

type validationError struct{ Field, Msg string }

func (e *validationError) Error() string { return e.Field + ": " + e.Msg }

// errChain 第4节：%w 成链、%v 断链、Is/As 判定、Join 成树。
func errChain() {
	// %w 保留链，%v 只是拼字符串
	wrapped := fmt.Errorf("open db: %w", errConnRefused)
	broken := fmt.Errorf("open db: %v", errConnRefused)
	fmt.Printf("%%w 包装: errors.Is=%v；%%v 拼接: errors.Is=%v（链断了）\n",
		errors.Is(wrapped, errConnRefused), errors.Is(broken, errConnRefused))

	// As 取结构化信息
	verr := &validationError{Field: "age", Msg: "must be positive"}
	chained := fmt.Errorf("validate user: %w", verr)
	var target *validationError
	if errors.As(chained, &target) {
		fmt.Printf("errors.As 取出: field=%s msg=%s（类型判定不依赖错误文本）\n", target.Field, target.Msg)
	}

	// Join 多错误（Go 1.20+）：树形遍历
	joined := errors.Join(errConnRefused, chained)
	fmt.Printf("errors.Join: Is(errConnRefused)=%v, As(validationError)=%v（遍历整棵错误树）\n",
		errors.Is(joined, errConnRefused), errors.As(joined, &target))

	// 反面教材：字符串判断
	fmt.Printf("反面教材: strings.Contains(err, \"refused\")=%v —— 消息改版/本地化即失效，禁用\n",
		strings.Contains(broken.Error(), "refused"))

	// 工程高频：net.Error 超时判定（自定义实现 net.Error 的超时错误）
	wrappedTimeout := fmt.Errorf("rpc call: %w", timeoutError{})
	var netErr net.Error
	if errors.As(wrappedTimeout, &netErr) && netErr.Timeout() {
		fmt.Println("errors.As(net.Error) 且 Timeout()=true → 判定超时，可重试")
	}
	fmt.Printf("对照: errConnRefused 不是 net.Error → errors.As=%v（As 沿链找不到该类型）\n",
		errors.As(wrapped, &netErr))
	fmt.Println("ctx 超时则用哨兵: errors.Is(err, context.DeadlineExceeded)")
}

// timeoutError 实现 net.Error 的超时错误（net.Error = error + Timeout/Temporary）。
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// panicRecover 第5节：recover 的位置限制与 goroutine 边界。
func panicRecover() {
	// 1) 有效形式：recover 在 defer 的函数体内直接调用
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("有效形式: defer func(){ recover() } 拦截到 %v\n", r)
			}
		}()
		panic("boom-1")
	}()

	// 2) 无效形式（会崩进程，不真跑，只说明）：
	//    a. defer helper()，helper 里调 recover —— recover 不是被 defer 的函数直接调用，返回 nil 且拦不住
	//    b. 函数体内裸写 recover() —— 没有在展开中的 panic，永远返回 nil
	//    c. 子 goroutine panic —— recover 只救当前 goroutine，外层 defer 再完美也拦不住
	fmt.Println("无效形式: ①defer 调用的普通函数里 recover ②函数体裸 recover ③跨 goroutine——均拦不住，进程退出")

	// 3) 标准生产姿势：长生命周期 goroutine 入口 recover + 带栈日志
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				fmt.Printf("生产姿势: recover=%v，栈长度=%d 字节（log 时拼上 debug.Stack()）\n", r, len(stack))
			}
		}()
		panicInner()
	}()
	fmt.Println("net/http handler 已内置 recover；自己 go 出去的 goroutine 入口必须自己包")
}

// panicInner 抛出 panic 供外层 recover 演示（直接调用，非 goroutine）。
func panicInner() {
	panic("boom-2")
}

// deferTraps 第6节：参数立即求值 + 命名返回值改写。
func deferTraps() {
	// 1) 参数在入 defer 链时求值拷贝
	i := 0
	defer fmt.Printf("陷阱1 裸调用: i=%d（入链时已拷贝）\n", i)
	i = 1
	defer func() {
		fmt.Printf("陷阱1 闭包版: i=%d（执行时才读）\n", i)
	}()

	// 2) 命名返回值可被 defer 修改（recover 转 error 的标准姿势）
	err := safeCall()
	fmt.Printf("陷阱2 命名返回值: safeCall 返回 error=%v（panic 被 defer 转成 error）\n", err)
}

// safeCall 演示 recover → 命名返回值 error。
func safeCall() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered: %v", r) // 改写的就是返回值
		}
	}()
	panic("inner")
}
