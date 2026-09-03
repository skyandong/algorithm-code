// # 错误处理模式实验（笔记 07）
//
// 对应笔记：notes/design-pattern/07-错误处理即模式.md
//
// 运行：go run ./experiments/ errors
//
// 实验项：
//
//	第1节：三分法 —— 哨兵 / 结构化类型 / 即时包装的创建与判定
//	第2节：错误只处理一次 —— 中间层加语境上抛, 顶层唯一一次日志
//	第3节：临时性判定 —— net.Error.Timeout + Canceled 绝不重试
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// RunErrorsExperiments 演示笔记 07 的错误处理模式。
func RunErrorsExperiments() {
	fmt.Println("========== 第1节: 三分法 ==========")
	e1ThreeForms()

	fmt.Println("\n========== 第2节: 只处理一次 ==========")
	e2HandleOnce()

	fmt.Println("\n========== 第3节: 重试边界 ==========")
	e3Retryable()
}

// ---------- 第1节：三分法 ----------

var ErrNotFound = errors.New("not found") // 形态一：哨兵

type ValidationError struct{ Field, Rule string } // 形态二：结构化

func (e *ValidationError) Error() string { return e.Field + ": " + e.Rule }

func loadUser(id int) error { // 形态三：即时包装（动词+实体+参数）
	if id == 404 {
		return fmt.Errorf("load user %d: %w", id, ErrNotFound)
	}
	if id < 0 {
		return fmt.Errorf("load user %d: %w", id, &ValidationError{Field: "id", Rule: "must be positive"})
	}
	return nil
}

func e1ThreeForms() {
	// 判定一：Is——是哪种错
	err := loadUser(404)
	fmt.Printf("Is(ErrNotFound)=%v（哨兵穿透包装链）\n", errors.Is(err, ErrNotFound))

	// 判定二：As——取结构化字段
	err = loadUser(-1)
	var ve *ValidationError
	if errors.As(err, &ve) {
		fmt.Printf("As(ValidationError): field=%s rule=%s（类型判定不依赖文本）\n", ve.Field, ve.Rule)
	}

	// 判定三：文本只给日志看
	fmt.Printf("日志排障视角: %v（动词+实体+参数, 0.5 秒定位）\n", err)
}

// ---------- 第2节：只处理一次 ----------

var logBuf strings.Builder // 收集"日志"，验证只打了一次

func repoSave(id int) error { return fmt.Errorf("exec sql: %w", timeoutError{}) }

func serviceSave(id int) error { // 中间层：只加语境，零日志
	if err := repoSave(id); err != nil {
		return fmt.Errorf("save order %d: %w", id, err)
	}
	return nil
}

func handlerSave(id int) { // 顶层：唯一一次日志 + 对外语义
	err := serviceSave(id)
	if err == nil {
		return
	}
	fmt.Fprintf(&logBuf, "[ERROR] save order failed: err=%v\n", err) // 唯一一次

	var ve *ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		fmt.Println("  → 对外 404")
	case errors.As(err, &ve):
		fmt.Println("  → 对外 400：" + ve.Field)
	default:
		fmt.Println("  → 对外 500（细节进日志, 不外泄）")
	}
}

func e2HandleOnce() {
	handlerSave(1)
	fmt.Printf("三层调用后日志条数: %d（若中间层也打, 这里会是 3 条告警噪音）\n",
		strings.Count(logBuf.String(), "[ERROR]"))
	fmt.Println("规则: 中间层只包不记, 顶层记一次+定对外语义")
}

// ---------- 第3节：重试边界 ----------

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func classify(err error) string { // 分类器：决定重试策略
	if errors.Is(err, context.Canceled) {
		return "取消——绝不重试, 上抛" // 方向信号不是失败
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "超时——看是哪个 ctx 超的, 谨慎重试"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "网络超时——可重试（幂等前提+指数退避+上限）"
	}
	return "未知——记日志人工看"
}

func e3Retryable() {
	wrapped := fmt.Errorf("rpc call user-svc: %w", timeoutError{})
	fmt.Printf("超时错误: %s\n", classify(wrapped))

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	fmt.Printf("取消错误: %s\n", classify(context.Canceled))
	_ = cancelled
}
