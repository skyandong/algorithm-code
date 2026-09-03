package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSymmetric(t *testing.T) {
	// LeetCode 示例 1: [1,2,2,3,4,4,3] → true
	t1 := &TreeNode{Val: 1,
		Left: &TreeNode{Val: 2,
			Left:  &TreeNode{Val: 3},
			Right: &TreeNode{Val: 4},
		},
		Right: &TreeNode{Val: 2,
			Left:  &TreeNode{Val: 4},
			Right: &TreeNode{Val: 3},
		},
	}
	assert.True(t, isSymmetric(t1))

	// LeetCode 示例 2: [1,2,2,null,3,null,3] → false
	t2 := &TreeNode{Val: 1,
		Left: &TreeNode{Val: 2,
			Right: &TreeNode{Val: 3},
		},
		Right: &TreeNode{Val: 2,
			Right: &TreeNode{Val: 3},
		},
	}
	assert.False(t, isSymmetric(t2))

	// 空树
	assert.True(t, isSymmetric(nil))

	// 单节点
	assert.True(t, isSymmetric(&TreeNode{Val: 1}))

	// 两子对称
	assert.True(t, isSymmetric(&TreeNode{Val: 1, Left: &TreeNode{Val: 2}, Right: &TreeNode{Val: 2}}))

	// 两子不对称(值不同)
	assert.False(t, isSymmetric(&TreeNode{Val: 1, Left: &TreeNode{Val: 2}, Right: &TreeNode{Val: 3}}))

	// 仅左子 / 仅右子
	assert.False(t, isSymmetric(&TreeNode{Val: 1, Left: &TreeNode{Val: 2}}))
	assert.False(t, isSymmetric(&TreeNode{Val: 1, Right: &TreeNode{Val: 2}}))

	// 深层对称: [1,2,2,3,null,null,3]
	t3 := &TreeNode{Val: 1,
		Left:  &TreeNode{Val: 2, Left: &TreeNode{Val: 3}},
		Right: &TreeNode{Val: 2, Right: &TreeNode{Val: 3}},
	}
	assert.True(t, isSymmetric(t3))

	// 深层不对称: [1,2,2,3,null,null,4]
	t4 := &TreeNode{Val: 1,
		Left:  &TreeNode{Val: 2, Left: &TreeNode{Val: 3}},
		Right: &TreeNode{Val: 2, Right: &TreeNode{Val: 4}},
	}
	assert.False(t, isSymmetric(t4))
}
