// # TDD 实验（笔记 14）
//
// 对应笔记：notes/golang/14-TDD测试驱动开发.md
//
// 运行：go run ./experiments/ tdd
//
// 实验项：
//
//	第1节：红-绿-重构——SplitBill 从最小实现到边界校验，测试逼出每个分支
//	第2节：表驱动 mini 跑器——用例名即规格，错误用例一等公民
//	第3节：手写 stub——消费者侧接口 + 记录调用 + 注入失败路径
package main

import (
	"errors"
	"fmt"
)

// RunTDDExperiments 演示笔记 14 的 TDD 循环与替身。
func RunTDDExperiments() {
	fmt.Println("========== 第1节: 红-绿-重构 ==========")
	t1RedGreenRefactor()

	fmt.Println("\n========== 第2节: 表驱动 mini 跑器 ==========")
	t2TableRunner()

	fmt.Println("\n========== 第3节: 手写 stub 替身 ==========")
	t3Stub()
}

// ---------- 第1节：红-绿-重构 ----------

// splitBillV1 绿1 的最小实现：只满足第一个测试（整除），没被要求的健壮性一个不写。
func splitBillV1(total, people int) (int, error) {
	return total / people, nil
}

var (
	tddErrPeople   = errors.New("人数必须为正")
	tddErrNegative = errors.New("总额不能为负")
)

// splitBillV2 绿2：红2 的用例（零人除零 panic）逼出了校验分支——每个分支有测试背书。
func splitBillV2(total, people int) (int, error) {
	if people <= 0 {
		return 0, fmt.Errorf("%w: %d", tddErrPeople, people)
	}
	if total < 0 {
		return 0, fmt.Errorf("%w: %d", tddErrNegative, total)
	}
	return total / people, nil
}

func t1RedGreenRefactor() {
	// 红1：先有测试才有签名——SplitBill(10000, 4) 想要 2500
	if got, _ := splitBillV1(10000, 4); got == 2500 {
		fmt.Println("绿1: splitBillV1 最小实现通过 happy path, 10000/4 = 2500")
	}

	// 红2：加边界用例 —— people=0 整数除零 panic，正是「红」
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("红2: splitBillV1(100, 0) → panic(%v) —— 新用例失败, 任务清单出现\n", r)
			}
		}()
		splitBillV1(100, 0)
	}()

	if got, _ := splitBillV2(10000, 4); got == 2500 {
		fmt.Println("绿2: splitBillV2 加校验后 happy path 依然绿 —— 重构有保护网")
	}
	if _, err := splitBillV2(100, 0); errors.Is(err, tddErrPeople) {
		fmt.Printf("绿2: 零人分支被测试逼出来 —— %v\n", err)
	}
	if _, err := splitBillV2(-1, 4); errors.Is(err, tddErrNegative) {
		fmt.Printf("绿2: 负总额分支同样有测试背书 —— %v\n", err)
	}
}

// ---------- 第2节：表驱动 mini 跑器 ----------

// t2TableRunner 在实验（非 go test）语境里演示表驱动的形态：
// 用例名是规格、错误用例一等公民。真正的 go test 版在 14_tdd_test.go。
func t2TableRunner() {
	tests := []struct {
		name    string
		total   int
		people  int
		want    int
		wantErr error
	}{
		{"整除", 10000, 4, 2500, nil},
		{"不整除向下取整", 10001, 4, 2500, nil},
		{"零人报错", 100, 0, 0, tddErrPeople},
		{"负人报错", 100, -2, 0, tddErrPeople},
		{"负总额报错", -1, 4, 0, tddErrNegative},
	}
	// 同一张规格表，先对 v1 跑（红），再对 v2 跑（绿）——测试不动，实现换掉
	runTable := func(label string, fn func(int, int) (int, error)) {
		pass := 0
		for _, tt := range tests {
			got, err, panicked := func() (g int, e error, p bool) {
				defer func() {
					if r := recover(); r != nil {
						e = fmt.Errorf("panic: %v", r) // panic 也是「红」的一种形态
						p = true
					}
				}()
				g, e = fn(tt.total, tt.people)
				return g, e, false
			}()
			ok := !panicked && got == tt.want && errors.Is(err, tt.wantErr)
			status := "PASS"
			if !ok {
				status = "FAIL"
			} else {
				pass++
			}
			fmt.Printf("  [%s] %-14s got=(%d, %v)\n", status, tt.name, got, err)
		}
		fmt.Printf("表驱动[%s]: %d/%d 用例通过 —— 断言可观察行为, 错误用 errors.Is 对哨兵\n", label, pass, len(tests))
	}

	fmt.Println("对 v1 跑同规格表（边界用例尚未被满足 → 红）:")
	runTable("v1", splitBillV1)
	fmt.Println("对 v2 跑同规格表（校验分支被测试逼出来 → 绿）:")
	runTable("v2", splitBillV2)
}

// ---------- 第3节：手写 stub ----------

// Notifier 消费者侧接口：按需定义最小视角（design-pattern/01 的直接应用）。
type Notifier interface {
	Paid(orderID string, cents int64) error
}

// tddSettleService 被测对象：只依赖最小接口，替身随插随换。
type tddSettleService struct {
	notify Notifier
}

func (s *tddSettleService) Settle(orderID string, cents int64) error {
	if err := s.notify.Paid(orderID, cents); err != nil {
		return fmt.Errorf("通知失败: %w", err)
	}
	return nil
}

// tddStubNotifier 手写 stub：记录调用供断言 + 可注入失败，3 行逻辑零依赖。
type tddStubNotifier struct {
	calls []string
	err   error
}

func (s *tddStubNotifier) Paid(orderID string, cents int64) error {
	s.calls = append(s.calls, fmt.Sprintf("%s:%d", orderID, cents))
	return s.err
}

func t3Stub() {
	// 用法一：成功路径 —— 断言交互内容（调了谁、参数对不对）
	stub := &tddStubNotifier{}
	svc := &tddSettleService{notify: stub}
	if err := svc.Settle("o1", 2500); err != nil {
		fmt.Println("stub: 意外失败:", err)
		return
	}
	fmt.Printf("stub: 成功路径, 记录到的调用 %v —— 交互内容可断言\n", stub.calls)

	// 用法二：失败路径 —— 注入错误，不用真连一个会失败的下游
	failing := &tddStubNotifier{err: errors.New("下游超时")}
	err := (&tddSettleService{notify: failing}).Settle("o2", 3000)
	fmt.Printf("stub: 注入失败路径 —— %v\n", err)

	// 顺手演示时钟注入：测试里时间也是确定的
	now := func() string { return "2026-09-08T18:00:00+08:00" }
	fmt.Printf("stub: 注入时钟 now() = %s —— 永不 sleep, 永不裸调 time.Now\n", now())
}
