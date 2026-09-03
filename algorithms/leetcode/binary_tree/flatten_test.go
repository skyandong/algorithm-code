package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// walkRight 沿 Right 走收集值,并校验所有 Left 为 nil
func walkRight(head *TreeNode) (vals []int, leftAllNil bool) {
	leftAllNil = true
	for head != nil {
		if head.Left != nil {
			leftAllNil = false
		}
		vals = append(vals, head.Val)
		head = head.Right
	}
	return
}

func TestFlatten(t *testing.T) {
	// LeetCode 示例: [1,2,5,3,4,null,6] → 1->2->3->4->5->6
	t1 := &TreeNode{Val: 1,
		Left:  &TreeNode{Val: 2, Left: &TreeNode{Val: 3}, Right: &TreeNode{Val: 4}},
		Right: &TreeNode{Val: 5, Right: &TreeNode{Val: 6}},
	}
	flatten(t1)
	vals, leftNil := walkRight(t1)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, vals)
	assert.True(t, leftNil, "Left 应全为 nil")

	// 空树(不 panic)
	flatten(nil)

	// 单节点
	t2 := &TreeNode{Val: 0}
	flatten(t2)
	vals, leftNil = walkRight(t2)
	assert.Equal(t, []int{0}, vals)
	assert.True(t, leftNil)

	// 左斜链
	t3 := &TreeNode{Val: 1, Left: &TreeNode{Val: 2, Left: &TreeNode{Val: 3}}}
	flatten(t3)
	vals, leftNil = walkRight(t3)
	assert.Equal(t, []int{1, 2, 3}, vals)
	assert.True(t, leftNil)

	// 右斜链(已展开)
	t4 := &TreeNode{Val: 1, Right: &TreeNode{Val: 2, Right: &TreeNode{Val: 3}}}
	flatten(t4)
	vals, leftNil = walkRight(t4)
	assert.Equal(t, []int{1, 2, 3}, vals)
	assert.True(t, leftNil)

	// 两节点(仅左子)
	t5 := &TreeNode{Val: 1, Left: &TreeNode{Val: 2}}
	flatten(t5)
	vals, leftNil = walkRight(t5)
	assert.Equal(t, []int{1, 2}, vals)
	assert.True(t, leftNil)
}
