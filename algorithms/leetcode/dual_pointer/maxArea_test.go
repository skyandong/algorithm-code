package dualpointer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxArea(t *testing.T) {
	// LeetCode 示例 1: [1,8] 之间,min(1,8)×7=49
	assert.Equal(t, 49, maxArea([]int{1, 8, 6, 2, 5, 4, 8, 3, 7}))

	// LeetCode 示例 2: 两根等高 1,1×1=1
	assert.Equal(t, 1, maxArea([]int{1, 1}))

	// 全等高:最宽 5×3=15
	assert.Equal(t, 15, maxArea([]int{5, 5, 5, 5}))

	// 递增:最优在 [3,5] 间,3×2=6
	assert.Equal(t, 6, maxArea([]int{1, 2, 3, 4, 5}))

	// 递减:对称,6
	assert.Equal(t, 6, maxArea([]int{5, 4, 3, 2, 1}))

	// [2,3,10,5,7,8]:最优在 [10,8] 间,min(10,8)×3=24
	assert.Equal(t, 24, maxArea([]int{2, 3, 10, 5, 7, 8}))

	// 两元素
	assert.Equal(t, 1, maxArea([]int{1, 2}))
}
