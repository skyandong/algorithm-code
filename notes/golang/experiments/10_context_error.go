// # Context 与错误处理实验
//
// 对应笔记：notes/golang/09-context与错误处理.md
//
// 本实验为断言式单元测试（无打印），断言见 10_context_error_test.go：
//
//	go test -run 'TestCancel|TestChildTimeout|TestDone|TestWithoutCancel|TestError|TestPanic|TestBareRecover|TestDefer|TestSafeCall' ./experiments/
//
// —— runtime 源码对照 ——
//
// src/context/context.go（可取消 ctx 的核心字段，简化示意）
//
//	type cancelCtx struct {
//		Context             // 内嵌父 ctx——取消沿这条链向上根传播
//		mu       sync.Mutex
//		done     atomic.Value          // Done() 返回的 channel，惰性创建
//		children map[canceler]struct{} // 直接子节点：cancel 时逐个关闭
//		err      atomic.Value          // 取消原因（context.Canceled 等），atomic 包装
//		cause    error                 // WithCancelCause 的更细取消原因
//	}
package main

import (
	"errors"
	"fmt"
	"net"
)

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
