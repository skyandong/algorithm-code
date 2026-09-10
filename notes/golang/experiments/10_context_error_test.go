// 实验 10（context 与错误处理），断言验证
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

// validationError 结构化错误：用 errors.As 取字段。
type validationError struct{ Field, Msg string }

func (e *validationError) Error() string { return e.Field + ": " + e.Msg }

// timeoutError 实现 net.Error 的超时错误（net.Error = error + Timeout/Temporary）。
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
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

	assert.ErrorIs(t, childA.Err(), context.Canceled)
	assert.ErrorIs(t, childB.Err(), context.Canceled, "子取消不影响父，父取消连坐子")
}

func TestChildTimeoutCannotExceedParent(t *testing.T) {
	p, pcancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer pcancel()
	c, ccancel := context.WithTimeout(p, 10*time.Second)
	defer ccancel()

	start := time.Now()
	<-c.Done()
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 3*time.Second, "Deadline 取更早者：子给 10s 也被父的 100ms 截断")
	assert.ErrorIs(t, c.Err(), context.DeadlineExceeded)
}

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

	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestWithoutCancelStripsCancelKeepsValue(t *testing.T) {
	type ctxKey struct{}

	parent, pcancel := context.WithTimeout(
		context.WithValue(context.Background(), ctxKey{}, "trace-42"), 50*time.Millisecond)
	defer pcancel()
	time.Sleep(80 * time.Millisecond) // 让父超时

	child := context.WithoutCancel(parent)
	assert.Nil(t, child.Done(), "WithoutCancel：Done()==nil，永不取消")
	assert.ErrorIs(t, parent.Err(), context.DeadlineExceeded)
	assert.Equal(t, "trace-42", child.Value(ctxKey{}), "只剥离取消，Value 照常继承")
}

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

func TestErrorWrappingWAndV(t *testing.T) {
	wrapped := fmt.Errorf("open db: %w", errConnRefused)
	broken := fmt.Errorf("open db: %v", errConnRefused)

	assert.ErrorIs(t, wrapped, errConnRefused, "%w 保留链")
	assert.NotErrorIs(t, broken, errConnRefused, "%v 只拼字符串，链断了")
}

func TestStringMatchingIsFragile(t *testing.T) {
	broken := fmt.Errorf("open db: %v", errConnRefused)

	assert.True(t, strings.Contains(broken.Error(), "refused"), "文本碰巧包含")
	assert.False(t, errors.Is(broken, errConnRefused),
		"但消息改版/本地化即失效——判定必须用 errors.Is/As，禁止字符串比对")
}

func TestErrorsAsStructured(t *testing.T) {
	verr := &validationError{Field: "age", Msg: "must be positive"}
	chained := fmt.Errorf("validate user: %w", verr)

	var target *validationError
	assert.True(t, errors.As(chained, &target), "As 沿链取结构化错误")
	assert.Equal(t, "age", target.Field)
	assert.Equal(t, "must be positive", target.Msg)
}

func TestErrorsJoinTree(t *testing.T) {
	verr := &validationError{Field: "age", Msg: "must be positive"}
	chained := fmt.Errorf("validate user: %w", verr)
	joined := errors.Join(errConnRefused, chained)

	assert.ErrorIs(t, joined, errConnRefused, "Join 成树，Is 遍历整棵")

	var target *validationError
	assert.True(t, errors.As(joined, &target), "As 同样遍历整棵树")
}

func TestErrorsAsNetError(t *testing.T) {
	wrappedTimeout := fmt.Errorf("rpc call: %w", timeoutError{})

	var netErr net.Error
	assert.True(t, errors.As(wrappedTimeout, &netErr))
	assert.True(t, netErr.Timeout(), "As(net.Error) + Timeout() → 可重试判定")

	wrapped := fmt.Errorf("open db: %w", errConnRefused)
	assert.False(t, errors.As(wrapped, &netErr), "哨兵错误不是 net.Error")
}

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
	assert.Contains(t, string(stack), "panicInner")
}

func TestBareRecoverReturnsNil(t *testing.T) {
	assert.Nil(t, recover(), "没有展开中的 panic，裸 recover 永远返回 nil")
}

func TestRecoverCrossGoroutineIsFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过子进程崩溃验证")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRecoverCrossGoroutineFatalChild", "-test.count=1")
	cmd.Env = append(os.Environ(), "CTX_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()

	assert.Error(t, err, "子 goroutine panic：外层 defer 拦不住，进程退出")
	assert.Contains(t, string(out), "panic: boom-child")
}

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

func TestSafeCallConvertsPanicToError(t *testing.T) {
	err := safeCall()
	assert.Error(t, err, "命名返回值可被 defer 改写")
	assert.Contains(t, err.Error(), "recovered: inner")
}

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
