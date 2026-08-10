package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvertTree(t *testing.T) {
	// LeetCode 示例: [4,2,7,1,3,6,9] → [4,7,2,9,6,3,1]
	root := &TreeNode{Val: 4,
		Left: &TreeNode{Val: 2,
			Left:  &TreeNode{Val: 1},
			Right: &TreeNode{Val: 3},
		},
		Right: &TreeNode{Val: 7,
			Left:  &TreeNode{Val: 6},
			Right: &TreeNode{Val: 9},
		},
	}
	expected := &TreeNode{Val: 4,
		Left: &TreeNode{Val: 7,
			Left:  &TreeNode{Val: 9},
			Right: &TreeNode{Val: 6},
		},
		Right: &TreeNode{Val: 2,
			Left:  &TreeNode{Val: 3},
			Right: &TreeNode{Val: 1},
		},
	}
	assert.Equal(t, expected, invertTree(root))

	// 空树
	assert.Nil(t, invertTree(nil))

	// 单节点
	assert.Equal(t, &TreeNode{Val: 1}, invertTree(&TreeNode{Val: 1}))

	// 仅左子: [1,2] → [1,null,2]
	leftOnly := &TreeNode{Val: 1, Left: &TreeNode{Val: 2}}
	assert.Equal(t, &TreeNode{Val: 1, Right: &TreeNode{Val: 2}}, invertTree(leftOnly))

	// 仅右子: [1,null,2] → [1,2]
	rightOnly := &TreeNode{Val: 1, Right: &TreeNode{Val: 2}}
	assert.Equal(t, &TreeNode{Val: 1, Left: &TreeNode{Val: 2}}, invertTree(rightOnly))

	// 平衡三层: [1,2,3] → [1,3,2]
	balanced := &TreeNode{Val: 1, Left: &TreeNode{Val: 2}, Right: &TreeNode{Val: 3}}
	assert.Equal(t, &TreeNode{Val: 1, Left: &TreeNode{Val: 3}, Right: &TreeNode{Val: 2}}, invertTree(balanced))
}
