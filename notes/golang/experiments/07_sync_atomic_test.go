// 实验 07（sync 锁与原子操作）的单元测试 —— 冒烟为主。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSyncSmoke 全量冒烟。
func TestSyncSmoke(t *testing.T) {
	requireDemoRun(t)

	out := captureStdout(t, RunSyncExperiments)
	assert.Contains(t, out, "他人 Unlock: 合法", "Mutex 无属主语义必须出现")
	assert.Contains(t, out, "RLock 被挡", "写锁闸门实验必须出现")
	assert.Contains(t, out, "配置快照", "atomic.Pointer 热更新必须出现")
	assert.Contains(t, out, "未 Reset 放回", "脏数据事故必须演示")
	assert.Contains(t, out, "自旋", "第 2 节自旋说明必须出现")
}
