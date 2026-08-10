package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeKLists(t *testing.T) {
	// LeetCode 示例 1
	assert.Equal(t, []int{1, 1, 2, 3, 4, 4, 5, 6},
		listVals(mergeKLists([]*ListNode{
			buildList(1, 4, 5), buildList(1, 3, 4), buildList(2, 6),
		})))

	// LeetCode 示例 2: 含空链表
	assert.Equal(t, []int{1}, listVals(mergeKLists([]*ListNode{
		buildList(), buildList(1),
	})))

	// 空切片
	assert.Nil(t, mergeKLists(nil))
	assert.Nil(t, mergeKLists([]*ListNode{}))

	// 全空链表
	assert.Nil(t, mergeKLists([]*ListNode{nil, nil, nil}))

	// 单链表
	assert.Equal(t, []int{1, 2, 3}, listVals(mergeKLists([]*ListNode{buildList(1, 2, 3)})))

	// 两链表
	assert.Equal(t, []int{1, 2, 3, 4}, listVals(mergeKLists([]*ListNode{
		buildList(1, 3), buildList(2, 4),
	})))

	// 每条单元素
	assert.Equal(t, []int{1, 2, 3, 4}, listVals(mergeKLists([]*ListNode{
		buildList(1), buildList(2), buildList(3), buildList(4),
	})))

	// 逆序散布
	assert.Equal(t, []int{1, 2, 3, 4}, listVals(mergeKLists([]*ListNode{
		buildList(4), buildList(3), buildList(2), buildList(1),
	})))

	// 含负数
	assert.Equal(t, []int{-2, -1, 1, 2}, listVals(mergeKLists([]*ListNode{
		buildList(-2, 1), buildList(-1, 2),
	})))

	// 重复
	assert.Equal(t, []int{1, 1, 1, 1}, listVals(mergeKLists([]*ListNode{
		buildList(1, 1), buildList(1, 1),
	})))

	// 混合空链表
	assert.Equal(t, []int{1, 2}, listVals(mergeKLists([]*ListNode{
		buildList(1), nil, buildList(2), nil,
	})))
}
