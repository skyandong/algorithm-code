package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLengthOfLIS(t *testing.T) {
	// LeetCode 示例 1: [2,3,7,101] 长度 4
	assert.Equal(t, 4, lengthOfLIS([]int{10, 9, 2, 5, 3, 7, 101, 18}))

	// LeetCode 示例 2: [0,1,2,3] 长度 4
	assert.Equal(t, 4, lengthOfLIS([]int{0, 1, 0, 3, 2, 3}))

	// LeetCode 示例 3: 全相同,严格递增只能取 1 个
	assert.Equal(t, 1, lengthOfLIS([]int{7, 7, 7, 7, 7}))

	// 空数组
	assert.Equal(t, 0, lengthOfLIS([]int{}))

	// 单元素
	assert.Equal(t, 1, lengthOfLIS([]int{5}))

	// 已升序
	assert.Equal(t, 4, lengthOfLIS([]int{1, 2, 3, 4}))

	// 已降序,只能取 1
	assert.Equal(t, 1, lengthOfLIS([]int{4, 3, 2, 1}))

	// 含负数
	assert.Equal(t, 4, lengthOfLIS([]int{-1, 0, 1, 0, 3}))

	// 两元素升/降
	assert.Equal(t, 2, lengthOfLIS([]int{1, 2}))
	assert.Equal(t, 1, lengthOfLIS([]int{2, 1}))
}
