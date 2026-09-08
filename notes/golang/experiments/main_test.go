// 实验入口（main.go）的单元测试
//
// 入口本身没什么业务逻辑，但入口表被改坏（重名 / 空 run）会直接让
// `go run ./experiments/ xxx` 静默失效，所以这里兜住。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEntriesTable 入口表：数量固定、名字唯一、run 非空。
func TestEntriesTable(t *testing.T) {
	assert.Len(t, entries, 14)

	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		assert.NotEmpty(t, e.name)
		assert.NotNil(t, e.run, "entry %q 的 run 不能为空", e.name)
		assert.False(t, seen[e.name], "entry %q 重复注册", e.name)
		seen[e.name] = true
	}
}

// TestUsage 用法输出里必须列出全部实验名，否则用户无从知道有哪些实验。
func TestUsage(t *testing.T) {
	out := captureStdout(t, usage)
	for _, e := range entries {
		assert.Contains(t, out, e.name)
	}
	assert.Contains(t, out, "all")
}
