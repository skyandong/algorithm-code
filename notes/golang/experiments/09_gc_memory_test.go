// 实验 09（内存管理与 GC）的单元测试 —— 冒烟为主。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGCMemorySmoke 全量冒烟。
func TestGCMemorySmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunGCMemoryExperiments)
	assert.Contains(t, out, "逃逸分析", "第 1 节典型场景必须出现")
	assert.Contains(t, out, "TotalAlloc", "MemStats 观察必须出现")
	assert.Contains(t, out, "sync.Pool 复用", "复用对比必须出现")
	assert.Contains(t, out, "GOMEMLIMIT", "软上限查询必须出现")
	assert.Contains(t, out, "三色标记", "GC 理论速览必须出现")
	assert.Contains(t, out, "gctrace", "GC 慢定位命令必须出现")
}
