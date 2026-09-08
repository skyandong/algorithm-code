// 实验入口（main.go）的单元测试
//
// 入口本身没什么业务逻辑，但案例表被改坏（重名 / 空 run / 漏描述）会直接让
// `go run ./experiments/ xxx` 静默失效，所以这里兜住。
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCasesTable 案例表：数量固定、名字唯一、run 与描述非空。
func TestCasesTable(t *testing.T) {
	assert.Len(t, cases, 11)

	seen := make(map[string]bool, len(cases))
	for _, c := range cases {
		assert.NotEmpty(t, c.name)
		assert.NotNil(t, c.run, "case %q 的 run 不能为空", c.name)
		assert.NotEmpty(t, c.desc, "case %q 缺描述", c.name)
		assert.False(t, seen[c.name], "case %q 重复注册", c.name)
		seen[c.name] = true
	}
}

// TestUsage 用法输出里必须列出全部案例名，否则用户无从知道有哪些实验。
func TestUsage(t *testing.T) {
	out := captureStdout(t, usage)
	for _, c := range cases {
		assert.Contains(t, out, c.name)
	}
	assert.Contains(t, out, "make up")
}
