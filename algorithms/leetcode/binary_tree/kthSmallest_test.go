package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKthSmallest(t *testing.T) {
	// 单节点
	root1 := &TreeNode{Val: 1}
	assert.Equal(t, 1, kthSmallest(root1, 1))

	// 左子树为空: [1,null,2]
	root2 := &TreeNode{
		Val: 1,
		Right: &TreeNode{
			Val: 2,
		},
	}
	assert.Equal(t, 1, kthSmallest(root2, 1))
	assert.Equal(t, 2, kthSmallest(root2, 2))

	// 右子树为空: [3,1,null]
	root3 := &TreeNode{
		Val: 3,
		Left: &TreeNode{
			Val: 1,
		},
	}
	assert.Equal(t, 1, kthSmallest(root3, 1))
	assert.Equal(t, 3, kthSmallest(root3, 2))

	// 示例: root = [3,1,4,null,2], k = 1 -> 1
	root4 := &TreeNode{
		Val: 3,
		Left: &TreeNode{
			Val: 1,
			Right: &TreeNode{
				Val: 2,
			},
		},
		Right: &TreeNode{
			Val: 4,
		},
	}
	assert.Equal(t, 1, kthSmallest(root4, 1))
	assert.Equal(t, 2, kthSmallest(root4, 2))
	assert.Equal(t, 3, kthSmallest(root4, 3))

	// 示例: root = [5,3,6,2,4,null,null,1], k = 3 -> 3
	root5 := &TreeNode{
		Val: 5,
		Left: &TreeNode{
			Val: 3,
			Left: &TreeNode{
				Val: 2,
				Left: &TreeNode{
					Val: 1,
				},
			},
			Right: &TreeNode{
				Val: 4,
			},
		},
		Right: &TreeNode{
			Val: 6,
		},
	}
	assert.Equal(t, 1, kthSmallest(root5, 1))
	assert.Equal(t, 3, kthSmallest(root5, 3))
	assert.Equal(t, 6, kthSmallest(root5, 6))
}
