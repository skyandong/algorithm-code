package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUniquePaths(t *testing.T) {
	// LeetCode 示例 1: 3x7 网格,28 条路径
	assert.Equal(t, 28, uniquePaths(3, 7))

	// LeetCode 示例 2: 3x2 网格,3 条路径
	assert.Equal(t, 3, uniquePaths(3, 2))

	// 单格
	assert.Equal(t, 1, uniquePaths(1, 1))

	// 单行
	assert.Equal(t, 1, uniquePaths(1, 5))

	// 单列
	assert.Equal(t, 1, uniquePaths(5, 1))

	// 2x2: 右下 or 下右
	assert.Equal(t, 2, uniquePaths(2, 2))

	// 2x3
	assert.Equal(t, 3, uniquePaths(2, 3))

	// 对称性:3x7 与 7x3 路径数相同
	assert.Equal(t, uniquePaths(3, 7), uniquePaths(7, 3))

	// 较大:C(18,9)=48620
	assert.Equal(t, 48620, uniquePaths(10, 10))
}
