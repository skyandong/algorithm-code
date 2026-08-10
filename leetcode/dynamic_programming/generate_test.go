package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerate(t *testing.T) {
	// LeetCode 示例: numRows=5
	assert.Equal(t,
		[][]int{{1}, {1, 1}, {1, 2, 1}, {1, 3, 3, 1}, {1, 4, 6, 4, 1}},
		generate(5))

	// 最小输入
	assert.Equal(t, [][]int{{1}}, generate(1))

	// 两行
	assert.Equal(t, [][]int{{1}, {1, 1}}, generate(2))

	// 三行
	assert.Equal(t, [][]int{{1}, {1, 1}, {1, 2, 1}}, generate(3))
}
