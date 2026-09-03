package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLongestValidParentheses(t *testing.T) {
	// LeetCode 示例 1: "(()" → 2
	assert.Equal(t, 2, longestValidParentheses("(()"))

	// LeetCode 示例 2: ")()())" → 4
	assert.Equal(t, 4, longestValidParentheses(")()())"))

	// 空串
	assert.Equal(t, 0, longestValidParentheses(""))

	// 简单配对
	assert.Equal(t, 2, longestValidParentheses("()"))

	// 嵌套
	assert.Equal(t, 6, longestValidParentheses("()(())"))
	assert.Equal(t, 6, longestValidParentheses("(()())"))

	// 并列
	assert.Equal(t, 4, longestValidParentheses("()()"))

	// 全是同一种
	assert.Equal(t, 0, longestValidParentheses("((("))
	assert.Equal(t, 0, longestValidParentheses(")))"))

	// 中途断开
	assert.Equal(t, 4, longestValidParentheses("(()))"))
	assert.Equal(t, 2, longestValidParentheses("()(()"))
	assert.Equal(t, 4, longestValidParentheses("(()()"))

	// 整串都有效
	assert.Equal(t, 8, longestValidParentheses("(()(()())"))
}
