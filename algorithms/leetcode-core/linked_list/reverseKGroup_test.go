package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReverseKGroup(t *testing.T) {
	// LeetCode 示例
	assert.Equal(t, []int{2, 1, 4, 3, 5},
		listVals(reverseKGroup(buildList(1, 2, 3, 4, 5), 2)))

	// k=3,剩 2 个不翻
	assert.Equal(t, []int{3, 2, 1, 4, 5},
		listVals(reverseKGroup(buildList(1, 2, 3, 4, 5), 3)))

	// k=1 不翻
	assert.Equal(t, []int{1, 2, 3}, listVals(reverseKGroup(buildList(1, 2, 3), 1)))

	// k=len 全翻
	assert.Equal(t, []int{3, 2, 1}, listVals(reverseKGroup(buildList(1, 2, 3), 3)))

	// k>len 原样返回(修复前会成环且返回 nil)
	assert.Equal(t, []int{1, 2, 3}, listVals(reverseKGroup(buildList(1, 2, 3), 5)))

	// 空
	assert.Nil(t, reverseKGroup(nil, 2))

	// 单元素
	assert.Equal(t, []int{1}, listVals(reverseKGroup(buildList(1), 1)))

	// 偶数长度 k=2 全翻
	assert.Equal(t, []int{2, 1, 4, 3}, listVals(reverseKGroup(buildList(1, 2, 3, 4), 2)))

	// k=4 剩 1 个
	assert.Equal(t, []int{4, 3, 2, 1, 5}, listVals(reverseKGroup(buildList(1, 2, 3, 4, 5), 4)))

	// 正好整除
	assert.Equal(t, []int{3, 2, 1, 6, 5, 4}, listVals(reverseKGroup(buildList(1, 2, 3, 4, 5, 6), 3)))
}
