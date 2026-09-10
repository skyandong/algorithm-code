// 实验 14（TDD 测试驱动开发），可复用实现（断言见 14_tdd_test.go）
// 对应笔记：notes/golang/13-TDD测试驱动开发.md
// 源码对照：src/testing/testing.go（TB 接口：Helper/Cleanup/Fatalf/Errorf/Skip）
//
//	go test -run 'TestSplitBill|TestStub' ./experiments/
package main

import (
	"errors"
	"fmt"
)

// splitBillV1 绿1 最小实现：只满足第一个测试（整除），未写任何健壮性。
func splitBillV1(total, people int) (int, error) {
	return total / people, nil
}

var (
	tddErrPeople   = errors.New("人数必须为正")
	tddErrNegative = errors.New("总额不能为负")
)

// splitBillV2 绿2：边界用例（零人除零 panic）逼出校验分支，每个分支有测试背书。
func splitBillV2(total, people int) (int, error) {
	if people <= 0 {
		return 0, fmt.Errorf("%w: %d", tddErrPeople, people)
	}
	if total < 0 {
		return 0, fmt.Errorf("%w: %d", tddErrNegative, total)
	}
	return total / people, nil
}

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

// tddStubNotifier 手写 stub：记录调用供断言 + 可注入失败，逻辑零依赖。
type tddStubNotifier struct {
	calls []string
	err   error
}

func (s *tddStubNotifier) Paid(orderID string, cents int64) error {
	s.calls = append(s.calls, fmt.Sprintf("%s:%d", orderID, cents))
	return s.err
}
