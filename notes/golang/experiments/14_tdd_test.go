// 实验 14（TDD 测试驱动开发），断言验证
// 对应笔记：notes/golang/13-TDD测试驱动开发.md
// 源码对照：src/testing/testing.go（TB 接口）
package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSplitBillV2 表驱动：happy path 与错误分支平起平坐（笔记第 4 节的 go test 版）。
func TestSplitBillV2(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		people  int
		want    int
		wantErr error
	}{
		{"整除", 10000, 4, 2500, nil},
		{"不整除向下取整", 10001, 4, 2500, nil},
		{"单人独吞", 100, 1, 100, nil},
		{"零人报错", 100, 0, 0, tddErrPeople},
		{"负人报错", 100, -2, 0, tddErrPeople},
		{"负总额报错", -1, 4, 0, tddErrNegative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitBillV2(tt.total, tt.people)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

// TestSplitBillV1Red v1 的「红」可复现：零人触发整数除零 panic（不可 recover 的运行时错误）。
func TestSplitBillV1Red(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "v1 未做校验, 零人除零必须 panic")
		assert.Contains(t, fmt.Sprint(r), "integer divide by zero",
			"panic 文本必须是整数除零")
	}()
	_, _ = splitBillV1(100, 0)
}

// TestStubNotifier 纯逻辑：stub 记录调用 + 失败注入 + 服务侧包装语境。
func TestStubNotifier(t *testing.T) {
	stub := &tddStubNotifier{}
	svc := &tddSettleService{notify: stub}

	require.NoError(t, svc.Settle("o1", 2500))
	assert.Equal(t, []string{"o1:2500"}, stub.calls, "成功路径必须记录交互内容")

	failing := &tddStubNotifier{err: errors.New("下游超时")}
	err := (&tddSettleService{notify: failing}).Settle("o2", 3000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "通知失败", "服务侧必须包上语境（笔记 09 篇）")
	assert.ErrorIs(t, err, failing.err, "%w 链必须可被 errors.Is 追溯")
}
