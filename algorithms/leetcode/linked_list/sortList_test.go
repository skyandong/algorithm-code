package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortList(t *testing.T) {
	// LeetCode 示例 1
	assert.Equal(t, []int{1, 2, 3, 4}, listVals(sortList(buildList(4, 2, 1, 3))))

	// LeetCode 示例 2
	assert.Equal(t, []int{-1, 0, 3, 4, 5}, listVals(sortList(buildList(-1, 5, 3, 4, 0))))

	// 空
	assert.Nil(t, sortList(nil))

	// 单元素
	assert.Equal(t, []int{1}, listVals(sortList(buildList(1))))

	// 两元素(有序/逆序)
	assert.Equal(t, []int{1, 2}, listVals(sortList(buildList(1, 2))))
	assert.Equal(t, []int{1, 2}, listVals(sortList(buildList(2, 1))))

	// 已排序
	assert.Equal(t, []int{1, 2, 3, 4, 5}, listVals(sortList(buildList(1, 2, 3, 4, 5))))

	// 逆序
	assert.Equal(t, []int{1, 2, 3, 4, 5}, listVals(sortList(buildList(5, 4, 3, 2, 1))))

	// 重复
	assert.Equal(t, []int{1, 1, 2, 3, 3}, listVals(sortList(buildList(3, 1, 2, 1, 3))))

	// 全相同
	assert.Equal(t, []int{1, 1, 1}, listVals(sortList(buildList(1, 1, 1))))

	// 全负数
	assert.Equal(t, []int{-5, -3, -2, -1}, listVals(sortList(buildList(-3, -1, -2, -5))))

	// 较大
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8},
		listVals(sortList(buildList(4, 2, 8, 5, 7, 1, 3, 6))))
}
