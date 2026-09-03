package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLongestPalindrome(t *testing.T) {
	// LeetCode 示例
	assert.Equal(t, "bab", longestPalindrome("babad"))
	assert.Equal(t, "bb", longestPalindrome("cbbd"))

	// 边界
	assert.Equal(t, "", longestPalindrome(""))
	assert.Equal(t, "a", longestPalindrome("a"))

	// 无长回文,取单字符
	assert.Equal(t, "a", longestPalindrome("ac"))

	// 全相同
	assert.Equal(t, "aaaa", longestPalindrome("aaaa"))

	// 整串回文
	assert.Equal(t, "aba", longestPalindrome("aba"))

	// 偶数长度回文
	assert.Equal(t, "aa", longestPalindrome("aa"))
	assert.Equal(t, "bb", longestPalindrome("abb"))

	// 较长
	assert.Equal(t, "anana", longestPalindrome("bananas"))
	assert.Equal(t, "aba", longestPalindrome("abac"))
}
