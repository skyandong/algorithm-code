// 实验 09（context 与错误处理），断言验证
// 对应笔记：notes/golang/09-context与错误处理.md
// 源码对照：src/context/context.go
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ===== 实验实现（原 10_context_error.go，已并入本文件）=====

// errConnRefused 哨兵错误：判定用 errors.Is，禁止比字符串。
var errConnRefused = errors.New("connection refused")

// errUpstreamForCause 取消原因：配合 WithCancelCause / Cause 使用。
var errUpstreamForCause = errors.New("upstream closed")

// heldCancel 承接 panic 用例里拿不到手的 cancel（那些用例在 With* 里就 panic 了），避免 vet 的 lostcancel 报警。
var heldCancel context.CancelFunc

// validationError 结构化错误：用 errors.As 取字段。
type validationError struct{ Field, Msg string }

// Error 实现 error 接口
func (e *validationError) Error() string { return e.Field + ": " + e.Msg }

// timeoutError 实现 net.Error 的超时错误（net.Error = error + Timeout/Temporary）。
type timeoutError struct{}

// Error 实现 error 接口
func (timeoutError) Error() string { return "i/o timeout" }

// Timeout 声明是超时错误（net.Error 的组成部分）
func (timeoutError) Timeout() bool { return true }

// Temporary 声明是临时错误（net.Error 的组成部分）
func (timeoutError) Temporary() bool { return true }

// panicInner 抛出 panic 供外层 recover 演示（直接调用，非 goroutine）。
func panicInner() {
	panic("boom-2")
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

var _ net.Error = timeoutError{}

// ===== 断言用例 =====

// TestCancelTreeBroadcast 第 1 节：父取消沿树广播，子取消不影响父
func TestCancelTreeBroadcast(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	childA, cancelA := context.WithCancel(parent)
	childB, cancelB := context.WithTimeout(parent, time.Hour)
	defer cancelA()
	defer cancelB()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-childA.Done() }()
	go func() { defer wg.Done(); <-childB.Done() }()

	cancel() // 父取消 → 两个子同时解除阻塞
	waitGroup(t, &wg, time.Second)

	assert.ErrorIs(t, childA.Err(), context.Canceled, "父取消沿树广播到子 A")
	assert.ErrorIs(t, childB.Err(), context.Canceled, "子取消不影响父，父取消连坐子")
}

// TestChildTimeoutCannotExceedParent 第 1 节：Deadline 取更早者
func TestChildTimeoutCannotExceedParent(t *testing.T) {
	p, pcancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer pcancel()
	c, ccancel := context.WithTimeout(p, 10*time.Second)
	defer ccancel()

	start := time.Now()
	<-c.Done()
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 3*time.Second, "Deadline 取更早者：子给 10s 也被父的 100ms 截断")
	assert.ErrorIs(t, c.Err(), context.DeadlineExceeded, "子 Deadline 被父截断而超时")
}

// TestDoneBroadcastMultipleWatchers 第 2 节：关闭是广播，多监听者同时解除阻塞
func TestDoneBroadcastMultipleWatchers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	const watchers = 3
	var woke sync.WaitGroup
	woke.Add(watchers)
	for i := 0; i < watchers; i++ {
		go func() {
			defer woke.Done()
			<-ctx.Done() // 关闭是广播：发送只能唤醒一个
		}()
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	waitGroup(t, &woke, time.Second)

	assert.ErrorIs(t, ctx.Err(), context.Canceled, "cancel 后 ctx.Err() 为 Canceled（广播已生效）")
}

// TestWithoutCancelStripsCancelKeepsValue 第 3 节：剥掉取消但保留 Value
func TestWithoutCancelStripsCancelKeepsValue(t *testing.T) {
	type ctxKey struct{}

	parent, pcancel := context.WithTimeout(
		context.WithValue(context.Background(), ctxKey{}, "trace-42"), 50*time.Millisecond)
	defer pcancel()

	select { // 等父真的超时：靠 sleep 猜时长，机器一忙 timer 晚点就假红
	case <-parent.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("父 ctx 未在预期时间内超时")
	}
	assert.ErrorIs(t, parent.Err(), context.DeadlineExceeded, "父确已超时，作为 WithoutCancel 的对照基线")

	child := context.WithoutCancel(parent)
	assert.Nil(t, child.Done(), "WithoutCancel：Done()==nil，永不取消")
	assert.Equal(t, "trace-42", child.Value(ctxKey{}), "只剥离取消，Value 照常继承")
}

// TestUncanceledChildDiesWithParent 第 3 节：忘 cancel 的子 ctx 随父一起死
func TestUncanceledChildDiesWithParent(t *testing.T) {
	root, rootCancel := context.WithCancel(context.Background())

	_, leakedCancel := context.WithTimeout(root, time.Hour) // ✗ 忘记 cancel
	child, childCancel := context.WithTimeout(root, time.Hour)
	defer childCancel()

	rootCancel()
	assert.ErrorIs(t, child.Err(), context.Canceled,
		"未摘除的子 ctx 挂在父上：父活多久它活多久，父取消则一起死")

	leakedCancel() // 收尾
}

// TestBackgroundIsEmptyRoot 第 1.1 节：Background/TODO 四个方法全是空壳，永不取消
func TestBackgroundIsEmptyRoot(t *testing.T) {
	bg := context.Background()

	assert.Nil(t, bg.Done(), "Background 的 Done() 是 nil channel（nil 上的接收永远阻塞 = 永不取消）")
	assert.NoError(t, bg.Err(), "Background 永不取消，Err() 恒为 nil")

	deadline, ok := bg.Deadline()
	assert.False(t, ok, "Background 没有截止时间")
	assert.True(t, deadline.IsZero(), "Background 的 Deadline 返回零值时间")

	assert.Nil(t, bg.Value(struct{}{}), "Background 挂不了值，Value() 恒为 nil")
	assert.NoError(t, context.Cause(bg), "未取消的 ctx，Cause 返回 nil")

	// String() 不在 Context 接口上（是具体类型的方法），靠 fmt 的 Stringer 分派取到
	assert.Equal(t, "context.Background", fmt.Sprintf("%v", bg), "Background 与 TODO 的唯一区别就是 String()")
	assert.Equal(t, "context.TODO", fmt.Sprintf("%v", context.TODO()), "两者行为完全一致，差别只是写给人看的意图")
}

// TestBackgroundBreaksCancelChain 第 1.1 节：拿 Background 顶替父 ctx 会切断取消链
func TestBackgroundBreaksCancelChain(t *testing.T) {
	parent, pcancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer pcancel()

	detached, dcancel := context.WithCancel(context.Background()) // ✗ 断链
	defer dcancel()
	attached, acancel := context.WithCancel(parent) // ✓ 挂父
	defer acancel()

	select { // 等父真的超时：不猜时长
	case <-parent.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("父 ctx 未在预期时间内超时")
	}

	assert.NoError(t, detached.Err(), "用 Background 起的子树收不到任何信号：父已超时，它仍认为未取消")
	assert.ErrorIs(t, attached.Err(), context.DeadlineExceeded, "挂父的子树被连坐，跟着超时")
}

// TestNilParentAndBadKeyPanic 第 1.1 节：不知道传什么也不能传 nil，会 panic
func TestNilParentAndBadKeyPanic(t *testing.T) {
	const nilParent = "cannot create context from nil parent"
	bg := context.Background()

	assert.PanicsWithValue(t, nilParent, func() { _, heldCancel = context.WithCancel(nil) }, "WithCancel(nil) panic")
	assert.PanicsWithValue(t, nilParent, func() { _ = context.WithoutCancel(nil) }, "WithoutCancel(nil) panic")
	assert.PanicsWithValue(t, nilParent, func() { _ = context.WithValue(nil, "k", 1) }, "WithValue(nil, ..) panic")

	assert.PanicsWithValue(t, "nil key", func() { _ = context.WithValue(bg, nil, 1) }, "key 为 nil 时 panic")
	assert.PanicsWithValue(t, "key is not comparable",
		func() { _ = context.WithValue(bg, []int{1}, 1) }, "key 不可比较（slice/map）时 panic")
}

// TestCauseCarriesReason 第 2.1 节：Err 只有两个哨兵，Cause 补「为什么被取消」
func TestCauseCarriesReason(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	child, childCancel := context.WithCancel(ctx)
	defer childCancel()

	cancel(errUpstreamForCause)

	assert.ErrorIs(t, ctx.Err(), context.Canceled, "Err() 仍是那两个哨兵之一，不受 cause 影响")
	assert.Equal(t, errUpstreamForCause, context.Cause(ctx), "Cause 拿到真正的原因")
	assert.Equal(t, errUpstreamForCause, context.Cause(child), "cause 沿树向下可见：子也能读到父的原因")
}

// TestCauseFirstCancelWins 第 2.1 节：父子各有 cause 时，先取消的那个生效
func TestCauseFirstCancelWins(t *testing.T) {
	errParent := errors.New("parent-cause")
	errChild := errors.New("child-cause")

	parent, pcancel := context.WithCancelCause(context.Background())
	child, ccancel := context.WithCancelCause(parent)

	ccancel(errChild) // 子先取消
	pcancel(errParent)

	assert.Equal(t, errChild, context.Cause(child), "子先被取消：子的 cause 是自己的")
	assert.Equal(t, errParent, context.Cause(parent), "父后取消：父的 cause 不受子影响")

	// WithoutCancel 与取消彻底脱钩，Cause 恒为 nil
	root, rootCancel := context.WithCancelCause(context.Background())
	rootCancel(errUpstreamForCause)
	assert.NoError(t, context.Cause(context.WithoutCancel(root)), "WithoutCancel 的 Cause 恒为 nil")
}

// TestCauseChildOnlyAfterParent 第 2.1 节：父先取消时，子的 Cause 跟随父
func TestCauseChildOnlyAfterParent(t *testing.T) {
	parent, pcancel := context.WithCancelCause(context.Background())
	child, ccancel := context.WithCancelCause(parent)
	defer ccancel(nil) // CancelCauseFunc 的签名是 func(cause error)，不设原因就传 nil

	pcancel(errUpstreamForCause) // 父先取消

	assert.Equal(t, errUpstreamForCause, context.Cause(child), "父先取消：子沿链读到父的 cause")
}

// TestErrNonNilImpliesDoneClosed 第 4.4 节：Err() 非 nil 时 Done() 必已关闭（源码内部先 <-Done()）
func TestErrNonNilImpliesDoneClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, ctx.Err(), "cancel 后 Err 非 nil")
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Err() 非 nil 但 Done() 未关闭：违反 Err 的 happens-before 保证")
	}
}

// TestAfterFuncStopCancelsCallback 第 2 节：AfterFunc 的 stop 能阻止回调，且 f 在自己的 goroutine 里跑
func TestAfterFuncStopCancelsCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan struct{}, 1)
	stop := context.AfterFunc(ctx, func() { ran <- struct{}{} })

	assert.True(t, stop(), "stop 返回 true：阻止了 f 运行")

	cancel()
	select {
	case <-ran:
		t.Fatal("stop 之后再 cancel，f 不应运行")
	case <-time.After(200 * time.Millisecond):
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	ran2 := make(chan struct{}, 1)
	context.AfterFunc(ctx2, func() { ran2 <- struct{}{} })
	cancel2()

	select {
	case <-ran2:
	case <-time.After(5 * time.Second):
		t.Fatal("AfterFunc 的 f 未在预期时间内执行")
	}
}

// TestErrorWrappingWAndV 第 6 节：%w 保留错误链，%v 断链
func TestErrorWrappingWAndV(t *testing.T) {
	wrapped := fmt.Errorf("open db: %w", errConnRefused)
	broken := fmt.Errorf("open db: %v", errConnRefused)

	assert.ErrorIs(t, wrapped, errConnRefused, "%w 保留链")
	assert.NotErrorIs(t, broken, errConnRefused, "%v 只拼字符串，链断了")
}

// TestStringMatchingIsFragile 第 6 节：字符串判错一改就失效
func TestStringMatchingIsFragile(t *testing.T) {
	broken := fmt.Errorf("open db: %v", errConnRefused)

	assert.True(t, strings.Contains(broken.Error(), "refused"), "文本碰巧包含")
	assert.False(t, errors.Is(broken, errConnRefused),
		"但消息改版/本地化即失效——判定必须用 errors.Is/As，禁止字符串比对")
}

// TestErrorsAsStructured 第 6 节：As 沿链取结构化错误
func TestErrorsAsStructured(t *testing.T) {
	verr := &validationError{Field: "age", Msg: "must be positive"}
	chained := fmt.Errorf("validate user: %w", verr)

	var target *validationError
	assert.True(t, errors.As(chained, &target), "As 沿链取结构化错误")
	assert.Equal(t, "age", target.Field, "As 取出的结构化错误字段可读")
	assert.Equal(t, "must be positive", target.Msg, "As 取出的结构化错误 Msg 字段可读")
}

// TestErrorsJoinTree 第 6 节：Join 成树，Is/As 遍历整棵
func TestErrorsJoinTree(t *testing.T) {
	verr := &validationError{Field: "age", Msg: "must be positive"}
	chained := fmt.Errorf("validate user: %w", verr)
	joined := errors.Join(errConnRefused, chained)

	assert.ErrorIs(t, joined, errConnRefused, "Join 成树，Is 遍历整棵")

	var target *validationError
	assert.True(t, errors.As(joined, &target), "As 同样遍历整棵树")
}

// TestErrorsAsNetError 第 6 节：As 出 net.Error 才能判超时
func TestErrorsAsNetError(t *testing.T) {
	wrappedTimeout := fmt.Errorf("rpc call: %w", timeoutError{})

	var netErr net.Error
	assert.True(t, errors.As(wrappedTimeout, &netErr), "As 能从包装错误中取出 net.Error")
	assert.True(t, netErr.Timeout(), "As(net.Error) + Timeout() → 可重试判定")

	wrapped := fmt.Errorf("open db: %w", errConnRefused)
	assert.False(t, errors.As(wrapped, &netErr), "哨兵错误不是 net.Error")
}

// TestPanicRecoverInDefer 第 7 节：recover 必须在 defer 体内直接调用
func TestPanicRecoverInDefer(t *testing.T) {
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		panic("boom-1")
	}()
	assert.Equal(t, "boom-1", recovered, "recover 必须在 defer 体内直接调用")

	var stack []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				stack = debug.Stack()
			}
		}()
		panicInner()
	}()
	assert.NotEmpty(t, stack, "生产姿势：recover 时拼 debug.Stack()")
	assert.Contains(t, string(stack), "panicInner", "debug.Stack 记录 panicInner 调用帧")
}

// TestBareRecoverReturnsNil 第 7 节：没有展开中的 panic，裸 recover 返回 nil
func TestBareRecoverReturnsNil(t *testing.T) {
	assert.Nil(t, recover(), "没有展开中的 panic，裸 recover 永远返回 nil")
}

// TestRecoverCrossGoroutineIsFatal 第 7 节：跨 goroutine 的 panic 拦不住（子进程验证）
func TestRecoverCrossGoroutineIsFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过子进程崩溃验证")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRecoverCrossGoroutineFatalChild", "-test.count=1")
	cmd.Env = append(os.Environ(), "CTX_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()

	assert.Error(t, err, "子 goroutine panic：外层 defer 拦不住，进程退出")
	assert.Contains(t, string(out), "panic: boom-child", "子进程输出含子 goroutine 的 panic")
}

// TestRecoverCrossGoroutineFatalChild 子进程实体：子 goroutine 内 panic
func TestRecoverCrossGoroutineFatalChild(t *testing.T) {
	if os.Getenv("CTX_FATAL_CHILD") != "1" {
		t.Skip("仅由 TestRecoverCrossGoroutineIsFatal 以子进程拉起")
	}

	defer func() { _ = recover() }() // 外层 recover 再完美也救不了别的 goroutine

	done := make(chan struct{})
	go func() {
		defer close(done)
		panic("boom-child")
	}()
	<-done
}

// TestDeferArgumentSnapshot 第 8 节：defer 参数入链时求值，闭包执行时才读
func TestDeferArgumentSnapshot(t *testing.T) {
	var captured []int

	i := 0
	func() {
		defer func(v int) { captured = append(captured, v) }(i) // 入 defer 链时求值
		i = 1
	}()
	assert.Equal(t, []int{0}, captured, "defer 参数在入链时拷贝")

	captured = nil
	j := 0
	func() {
		defer func() { captured = append(captured, j) }() // 闭包：执行时才读
		j = 1
	}()
	assert.Equal(t, []int{1}, captured, "闭包捕获变量本身，执行时读到新值")
}

// TestSafeCallConvertsPanicToError 第 7 节：panic 被 defer 转成 error
func TestSafeCallConvertsPanicToError(t *testing.T) {
	err := safeCall()
	assert.Error(t, err, "命名返回值可被 defer 改写")
	assert.Contains(t, err.Error(), "recovered: inner", "panic 被 defer 转写为 recovered 消息")
}

// waitGroup 带超时地等 wg 结束（避免测试挂死）
func waitGroup(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("等待超时")
	}
}

// ===== 实验实现：多 worker 查找 + context 取消（第 3 节）=====

// findWithTimeout 多协程查找目标值，找到即取消其余 worker，返回 "found"/"notfound"/"timeout"。
func findWithTimeout(values []int, target, workers int, timeout time.Duration) string {
	return searchValues(values, target, workers, timeout, 0)
}

// findSlowWithTimeout 同 findWithTimeout，但每元素检查前加微小延时，确定性触发超时取消路径。
func findSlowWithTimeout(values []int, target, workers int, timeout time.Duration) string {
	return searchValues(values, target, workers, timeout, 2*time.Microsecond)
}

// searchValues 多 worker 并发查找，带超时提前返回
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

// ===== 断言用例 =====

// TestFindWithTimeoutFound 第 3 节：目标存在 → 找到即取消其余 worker。
func TestFindWithTimeoutFound(t *testing.T) {
	big := make([]int, 1_000_000)
	for i := range big {
		big[i] = i
	}
	assert.Equal(t, "found", findWithTimeout(big, 777777, 8, 5*time.Second), "目标存在：找到即返回 found")
}

// TestFindWithTimeoutNotFound 第 3 节：目标不存在且扫描很快完成 → notfound（未超时）。
func TestFindWithTimeoutNotFound(t *testing.T) {
	big := make([]int, 1_000_000)
	for i := range big {
		big[i] = i
	}
	assert.Equal(t, "notfound", findWithTimeout(big, -1, 8, 5*time.Second), "目标不存在且扫描完成：返回 notfound")
}

// TestFindSlowWithTimeout 第 3 节：目标不存在 + 慢查询 → 超时取消 → timeout。
func TestFindSlowWithTimeout(t *testing.T) {
	big := make([]int, 1_000_000)
	for i := range big {
		big[i] = i
	}
	assert.Equal(t, "timeout", findSlowWithTimeout(big, -1, 2, 300*time.Millisecond), "目标不存在加慢查询：超时返回 timeout")
}
