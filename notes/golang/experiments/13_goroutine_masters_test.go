// 实验 13（名家并发模式）的单元测试 —— 冒烟为主。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMastersSmoke 全量冒烟。
func TestMastersSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunMastersExperiments)
	assert.Contains(t, out, "close(ch) 广播停止信号", "Dave Cheney 广播模式必须出现")
	assert.Contains(t, out, "nil channel 优雅多路等待", "nil channel 模式必须出现")
	assert.Contains(t, out, "Pipeline", "官方博客流水线必须出现")
	assert.Contains(t, out, "select + 超时", "超时模式必须出现")
}
