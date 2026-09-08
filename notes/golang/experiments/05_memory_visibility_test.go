// 实验 05（并发内存可见性）的单元测试 —— 冒烟为主（竞态本身靠 -race 观察，不进常规用例）。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestVisibilitySmoke 全量冒烟。
func TestVisibilitySmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunVisibilityExperiments)
	assert.Contains(t, out, "data race 演示", "第 1 节竞态演示必须出现")
	assert.Contains(t, out, "atomic 发布状态")
	assert.Contains(t, out, "happens-before 建立边", "规则速览必须出现")
	assert.Contains(t, out, "race detector", "-race 说明必须出现")
}
