// 实验 08（GMP 调度器）的单元测试 —— 冒烟为主。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGMPSmoke 全量冒烟。
func TestGMPSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunGMPExperiments)
	assert.Contains(t, out, "NumCPU", "P 的数量语义必须出现")
	assert.Contains(t, out, "Gosched", "主动让出必须出现")
	assert.Contains(t, out, "抢占", "异步抢占必须出现")
	assert.Contains(t, out, "findRunnable 找活优先级", "调度循环说明必须出现")
	assert.Contains(t, out, "netpoller", "网络 IO 不占 M 的说明必须出现")
}
