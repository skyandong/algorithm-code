package leetcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValid(t *testing.T) {
	// LeetCode 示例
	assert.True(t, isValid("()"))
	assert.True(t, isValid("()[]{}"))
	assert.False(t, isValid("(]"))
	assert.False(t, isValid("([)]"))
	assert.True(t, isValid("{[]}"))

	// 空串
	assert.True(t, isValid(""))

	// 未闭合 / 未匹配
	assert.False(t, isValid("("))
	assert.False(t, isValid(")"))
	assert.False(t, isValid("]"))
	assert.False(t, isValid("((("))
	assert.False(t, isValid(")))"))

	// 深层嵌套
	assert.True(t, isValid("(((((((((())))))))))"))

	// 复合
	assert.True(t, isValid("{()[]()}"))
	assert.False(t, isValid("{[}]"))
}
