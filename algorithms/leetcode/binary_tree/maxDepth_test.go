package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxDepth(t *testing.T) {
	// LeetCode 示例: [3,9,20,null,null,15,7] → 3
	tree := &TreeNode{Val: 3,
		Left: &TreeNode{Val: 9},
		Right: &TreeNode{Val: 20,
			Left:  &TreeNode{Val: 15},
			Right: &TreeNode{Val: 7},
		},
	}
	assert.Equal(t, 3, maxDepth(tree))

	// 空树
	assert.Equal(t, 0, maxDepth(nil))

	// 单节点
	assert.Equal(t, 1, maxDepth(&TreeNode{Val: 1}))

	// 左斜链
	leftLean := &TreeNode{Val: 1,
		Left: &TreeNode{Val: 2,
			Left: &TreeNode{Val: 3},
		},
	}
	assert.Equal(t, 3, maxDepth(leftLean))

	// 右斜链
	rightLean := &TreeNode{Val: 1,
		Right: &TreeNode{Val: 2,
			Right: &TreeNode{Val: 3},
		},
	}
	assert.Equal(t, 3, maxDepth(rightLean))

	// 平衡
	balanced := &TreeNode{Val: 1,
		Left:  &TreeNode{Val: 2},
		Right: &TreeNode{Val: 3},
	}
	assert.Equal(t, 2, maxDepth(balanced))

	// 深度不对称(左更深)
	asym := &TreeNode{Val: 1,
		Left: &TreeNode{Val: 2,
			Left: &TreeNode{Val: 4},
		},
		Right: &TreeNode{Val: 3},
	}
	assert.Equal(t, 3, maxDepth(asym))
}
