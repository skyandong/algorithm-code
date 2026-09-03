package dualpointer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThreeSum(t *testing.T) {
	// LeetCode 示例 1
	assert.ElementsMatch(t,
		[][]int{{-1, -1, 2}, {-1, 0, 1}},
		threeSum([]int{-1, 0, 1, 2, -1, -4}))

	// LeetCode 示例 2: 空数组
	assert.Empty(t, threeSum([]int{}))

	// LeetCode 示例 3: 不足 3 个
	assert.Empty(t, threeSum([]int{0}))

	// 三个 0
	assert.ElementsMatch(t, [][]int{{0, 0, 0}}, threeSum([]int{0, 0, 0}))

	// 四个 0:去重后只剩一个三元组
	assert.ElementsMatch(t, [][]int{{0, 0, 0}}, threeSum([]int{0, 0, 0, 0}))

	// 全正:剪枝返回空
	assert.Empty(t, threeSum([]int{1, 2, 3}))

	// 全负:剪枝返回空
	assert.Empty(t, threeSum([]int{-3, -2, -1}))

	// 不足 3 个
	assert.Empty(t, threeSum([]int{1, 2}))

	// 多个三元组
	assert.ElementsMatch(t,
		[][]int{{-2, -1, 3}, {-2, 0, 2}, {-1, 0, 1}},
		threeSum([]int{3, 0, -2, -1, 1, 2}))

	// 含重复元素:去重
	assert.ElementsMatch(t,
		[][]int{{-2, 0, 2}, {-2, 1, 1}},
		threeSum([]int{-2, 0, 1, 1, 2}))
}
