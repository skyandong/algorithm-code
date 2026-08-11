package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeTwoLists(t *testing.T) {
	// LeetCode 示例 1
	assert.Equal(t, []int{1, 1, 2, 3, 4, 4},
		listVals(mergeTwoLists(buildList(1, 2, 4), buildList(1, 3, 4))))

	// LeetCode 示例 2: 两边都空
	assert.Nil(t, mergeTwoLists(nil, nil))

	// LeetCode 示例 3: 一边空
	assert.Equal(t, []int{0}, listVals(mergeTwoLists(nil, buildList(0))))
	assert.Equal(t, []int{0}, listVals(mergeTwoLists(buildList(0), nil)))

	// 单元素各一
	assert.Equal(t, []int{1, 2}, listVals(mergeTwoLists(buildList(1), buildList(2))))
	assert.Equal(t, []int{1, 2}, listVals(mergeTwoLists(buildList(2), buildList(1))))

	// 交错
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6},
		listVals(mergeTwoLists(buildList(1, 3, 5), buildList(2, 4, 6))))

	// 全重复
	assert.Equal(t, []int{1, 1, 1, 1, 1, 1},
		listVals(mergeTwoLists(buildList(1, 1, 1), buildList(1, 1, 1))))

	// 含负数
	assert.Equal(t, []int{-3, -2, -1, 0},
		listVals(mergeTwoLists(buildList(-3, -1), buildList(-2, 0))))

	// 一边整体更小
	assert.Equal(t, []int{1, 2, 3, 4},
		listVals(mergeTwoLists(buildList(1, 2), buildList(3, 4))))

	// 单元素 vs 多元素
	assert.Equal(t, []int{1, 2, 3, 5},
		listVals(mergeTwoLists(buildList(5), buildList(1, 2, 3))))
}
