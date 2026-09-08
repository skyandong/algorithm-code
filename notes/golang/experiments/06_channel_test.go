// 实验 06（channel 与 nil 语义）的单元测试 —— 冒烟为主（全部语义都在打印演示里）。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestChannelSmoke 全量冒烟。
func TestChannelSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunChannelExperiments)
	assert.Contains(t, out, "nil channel 接收 case 永远不会就绪")
	assert.Contains(t, out, "ok=true", "已关闭 channel 先读完缓冲再返回 false")
	assert.Contains(t, out, "ok=false")
	assert.Contains(t, out, "发送返回耗时", "无缓冲直接交接的实测必须出现")
	assert.Contains(t, out, "close of closed channel", "双重 close 必须演示")
	assert.Contains(t, out, "send on closed channel", "已关再发必须演示")
	assert.Contains(t, out, "close of nil channel", "close(nil) 必须演示")
	assert.Contains(t, out, "状态转换表", "行为表必须出现")
}
