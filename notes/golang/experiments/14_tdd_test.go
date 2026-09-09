// 实验 14（TDD）的单元测试：纯逻辑用例 + demo 冒烟。
// 本文件自身就是笔记 14 的示范样本：表驱动 + errors.Is 断言 + -short 分层。
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

// TestSplitBillV1Red v1 的「红」可复现：零人触发整数除零 panic。
// 注意 runtime panic 值是 runtime.Error 而非 string，用 fmt.Sprint 比对文本。
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

// TestTDDSmoke 全量冒烟：跑一遍实验并断言关键结论输出。
func TestTDDSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunTDDExperiments)
	assert.Contains(t, out, "红2", "红步骤必须出现")
	assert.Contains(t, out, "integer divide by zero", "v1 的除零 panic 必须演示")
	assert.Contains(t, out, "[FAIL] 零人报错", "对 v1 跑表必须出现失败行（红）")
	assert.Contains(t, out, "表驱动[v1]: 2/5 用例通过", "v1 的红")
	assert.Contains(t, out, "表驱动[v2]: 5/5 用例通过", "v2 的绿")
	assert.Contains(t, out, "注入失败路径", "stub 失败注入必须出现")
}
