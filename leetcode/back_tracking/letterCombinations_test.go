package backtracking

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLetterCombinations(t *testing.T) {
	// LeetCode 示例 1
	assert.Equal(t, []string{"ad", "ae", "af", "bd", "be", "bf", "cd", "ce", "cf"},
		letterCombinations("23"))

	// LeetCode 示例 2: 空输入
	assert.Empty(t, letterCombinations(""))

	// 单数字 2(3 个字母)
	assert.Equal(t, []string{"a", "b", "c"}, letterCombinations("2"))

	// 7 有 4 个字母
	assert.Equal(t, []string{"p", "q", "r", "s"}, letterCombinations("7"))

	// 9 有 4 个字母
	assert.Equal(t, []string{"w", "x", "y", "z"}, letterCombinations("9"))

	// 三位数字: 3*3*3=27
	got := letterCombinations("234")
	assert.Len(t, got, 27)
	assert.Contains(t, got, "adg")
	assert.Contains(t, got, "cfi")

	// 两个 4 字母键: 4*4=16
	assert.Len(t, letterCombinations("79"), 16)
}
