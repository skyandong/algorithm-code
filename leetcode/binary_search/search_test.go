package binarysearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearch(t *testing.T) {
	// LeetCode 示例 1
	assert.Equal(t, 4, search([]int{4, 5, 6, 7, 0, 1, 2}, 0))

	// LeetCode 示例 2: 不存在
	assert.Equal(t, -1, search([]int{4, 5, 6, 7, 0, 1, 2}, 3))

	// LeetCode 示例 3: 单元素,找不到
	assert.Equal(t, -1, search([]int{1}, 0))

	// 单元素,找到
	assert.Equal(t, 0, search([]int{1}, 1))

	// 未旋转(普通有序)
	assert.Equal(t, 2, search([]int{1, 2, 3, 4, 5}, 3))
	assert.Equal(t, -1, search([]int{1, 2, 3, 4, 5}, 6))

	// 两元素
	assert.Equal(t, 1, search([]int{3, 1}, 1))
	assert.Equal(t, 0, search([]int{3, 1}, 3))
	assert.Equal(t, -1, search([]int{3, 1}, 2))
	assert.Equal(t, 1, search([]int{1, 3}, 3))

	// 较大旋转数组,查边界
	nums := []int{6, 7, 8, 1, 2, 3, 4, 5}
	assert.Equal(t, 0, search(nums, 6))
	assert.Equal(t, 2, search(nums, 8))
	assert.Equal(t, 7, search(nums, 5))
	assert.Equal(t, -1, search(nums, 9))
}
