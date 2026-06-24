package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidBST(t *testing.T) {
	// 空树
	assert.True(t, isValidBST(nil))

	// 单节点
	assert.True(t, isValidBST(&TreeNode{Val: 1}))

	// 合法BST: [2,1,3]
	root := &TreeNode{
		Val: 2,
		Left: &TreeNode{
			Val: 1,
		},
		Right: &TreeNode{
			Val: 3,
		},
	}
	assert.True(t, isValidBST(root))

	// 左子节点大于根节点: [1,2]
	root2 := &TreeNode{
		Val: 1,
		Left: &TreeNode{
			Val: 2,
		},
	}
	assert.False(t, isValidBST(root2))

	// 右子节点小于根节点: [1,null,2] 但右子节点是1
	root3 := &TreeNode{
		Val: 3,
		Right: &TreeNode{
			Val: 2,
		},
	}
	assert.False(t, isValidBST(root3))

	// 多层合法BST: [5,3,7,1,4,6,null]
	root4 := &TreeNode{
		Val: 5,
		Left: &TreeNode{
			Val: 3,
			Left: &TreeNode{
				Val: 1,
			},
			Right: &TreeNode{
				Val: 4,
			},
		},
		Right: &TreeNode{
			Val: 7,
			Left: &TreeNode{
				Val: 6,
			},
		},
	}
	assert.True(t, isValidBST(root4))

	// 左子树深层节点大于根: [10,5,null,null,15] — 15在左子树中但大于根10，非法
	root5 := &TreeNode{
		Val: 10,
		Left: &TreeNode{
			Val: 5,
			Right: &TreeNode{
				Val: 15,
			},
		},
	}
	assert.False(t, isValidBST(root5))

	// 右子树深层节点小于根: [10,null,20,15,25] — 15在右子树中但15<20满足，合法
	root6 := &TreeNode{
		Val: 10,
		Right: &TreeNode{
			Val: 20,
			Left: &TreeNode{
				Val: 15,
			},
			Right: &TreeNode{
				Val: 25,
			},
		},
	}
	assert.True(t, isValidBST(root6))

	// 右子树深层节点小于根: [10,null,20,9,null] — 9在右子树中但9<10，非法
	root7 := &TreeNode{
		Val: 10,
		Right: &TreeNode{
			Val: 20,
			Left: &TreeNode{
				Val: 9,
			},
		},
	}
	assert.False(t, isValidBST(root7))
}
