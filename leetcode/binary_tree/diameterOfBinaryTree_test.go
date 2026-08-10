package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiameterOfBinaryTree(t *testing.T) {
	// LeetCode 示例 1: [1,2,3,4,5] → 3
	t1 := &TreeNode{Val: 1,
		Left: &TreeNode{Val: 2,
			Left:  &TreeNode{Val: 4},
			Right: &TreeNode{Val: 5},
		},
		Right: &TreeNode{Val: 3},
	}
	assert.Equal(t, 3, diameterOfBinaryTree(t1))

	// LeetCode 示例 2: [1,2] → 1
	t2 := &TreeNode{Val: 1, Left: &TreeNode{Val: 2}}
	assert.Equal(t, 1, diameterOfBinaryTree(t2))

	// 空树
	assert.Equal(t, 0, diameterOfBinaryTree(nil))

	// 单节点(无边)
	assert.Equal(t, 0, diameterOfBinaryTree(&TreeNode{Val: 1}))

	// 左斜链 1-2-3-4 → 3 条边
	leftLean := &TreeNode{Val: 1, Left: &TreeNode{Val: 2, Left: &TreeNode{Val: 3, Left: &TreeNode{Val: 4}}}}
	assert.Equal(t, 3, diameterOfBinaryTree(leftLean))

	// 右斜链
	rightLean := &TreeNode{Val: 1, Right: &TreeNode{Val: 2, Right: &TreeNode{Val: 3, Right: &TreeNode{Val: 4}}}}
	assert.Equal(t, 3, diameterOfBinaryTree(rightLean))

	// 平衡满二叉树 [1,2,3,4,5,6,7] → 4
	t3 := &TreeNode{Val: 1,
		Left: &TreeNode{Val: 2,
			Left:  &TreeNode{Val: 4},
			Right: &TreeNode{Val: 5},
		},
		Right: &TreeNode{Val: 3,
			Left:  &TreeNode{Val: 6},
			Right: &TreeNode{Val: 7},
		},
	}
	assert.Equal(t, 4, diameterOfBinaryTree(t3))

	// [1,2,3,null,null,4,5] → 3
	t4 := &TreeNode{Val: 1,
		Left: &TreeNode{Val: 2},
		Right: &TreeNode{Val: 3,
			Left:  &TreeNode{Val: 4},
			Right: &TreeNode{Val: 5},
		},
	}
	assert.Equal(t, 3, diameterOfBinaryTree(t4))
}
