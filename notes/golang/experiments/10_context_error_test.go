// 实验 10（context 与错误处理）的单元测试 —— 冒烟为主。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestContextSmoke 全量冒烟。
func TestContextSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunContextExperiments)
	assert.Contains(t, out, "取消树广播", "取消树必须出现")
	assert.Contains(t, out, "Is/As/Join", "错误链必须出现")
	assert.Contains(t, out, "recover", "panic 边界必须出现")
	assert.Contains(t, out, "defer 求值时机", "defer 陷阱必须出现")
}
