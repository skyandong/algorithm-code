package arrays

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxSubArray(t *testing.T) {
	// LeetCode 示例 1: [4,-1,2,1] 和为 6
	assert.Equal(t, 6, maxSubArray([]int{-2, 1, -3, 4, -1, 2, 1, -5, 4}))

	// LeetCode 示例 2: 全取,23
	assert.Equal(t, 23, maxSubArray([]int{5, 4, -1, 7, 8}))

	// 单元素
	assert.Equal(t, 5, maxSubArray([]int{5}))

	// 单个负数
	assert.Equal(t, -5, maxSubArray([]int{-5}))

	// 全负:取最大的那个
	assert.Equal(t, -1, maxSubArray([]int{-1, -2, -3}))

	// 全正:全取
	assert.Equal(t, 10, maxSubArray([]int{1, 2, 3, 4}))

	// 两元素
	assert.Equal(t, 1, maxSubArray([]int{-2, 1}))
	assert.Equal(t, 1, maxSubArray([]int{1, -2}))
	assert.Equal(t, -1, maxSubArray([]int{-2, -1}))

	// 含 0
	assert.Equal(t, 0, maxSubArray([]int{0}))
	assert.Equal(t, 0, maxSubArray([]int{0, 0, 0}))

	// 负数中取最大单个
	assert.Equal(t, -1, maxSubArray([]int{-2, -1, -3}))
}
