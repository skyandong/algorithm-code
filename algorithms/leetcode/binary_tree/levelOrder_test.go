package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevelOrder(t *testing.T) {
	// LeetCode 示例 1: [3,9,20,null,null,15,7]
	t1 := &TreeNode{Val: 3,
		Left:  &TreeNode{Val: 9},
		Right: &TreeNode{Val: 20, Left: &TreeNode{Val: 15}, Right: &TreeNode{Val: 7}},
	}
	assert.Equal(t, [][]int{{3}, {9, 20}, {15, 7}}, levelOrder(t1))

	// 空树
	assert.Empty(t, levelOrder(nil))

	// 单节点
	assert.Equal(t, [][]int{{1}}, levelOrder(&TreeNode{Val: 1}))

	// 两层
	t2 := &TreeNode{Val: 1, Left: &TreeNode{Val: 2}, Right: &TreeNode{Val: 3}}
	assert.Equal(t, [][]int{{1}, {2, 3}}, levelOrder(t2))

	// 左斜链(每层一个)
	leftLean := &TreeNode{Val: 1, Left: &TreeNode{Val: 2, Left: &TreeNode{Val: 3}}}
	assert.Equal(t, [][]int{{1}, {2}, {3}}, levelOrder(leftLean))

	// 右斜链
	rightLean := &TreeNode{Val: 1, Right: &TreeNode{Val: 2, Right: &TreeNode{Val: 3}}}
	assert.Equal(t, [][]int{{1}, {2}, {3}}, levelOrder(rightLean))

	// [1,2,3,4,null,null,5]
	t3 := &TreeNode{Val: 1,
		Left:  &TreeNode{Val: 2, Left: &TreeNode{Val: 4}},
		Right: &TreeNode{Val: 3, Right: &TreeNode{Val: 5}},
	}
	assert.Equal(t, [][]int{{1}, {2, 3}, {4, 5}}, levelOrder(t3))
}
