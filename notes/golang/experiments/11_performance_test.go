// 实验 11（性能调优实战）的单元测试 —— 冒烟为主。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPerformanceSmoke 全量冒烟。
func TestPerformanceSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunPerformanceExperiments)
	assert.Contains(t, out, "预分配 vs 动态 append")
	assert.Contains(t, out, "累计分配", "分配量对比必须出现")
	assert.Contains(t, out, "值传递", "大结构体传值实验必须出现")
	assert.Contains(t, out, "线上排查标准路径", "速查必须出现")
}
