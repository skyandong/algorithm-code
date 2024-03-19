package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeTrees(t *testing.T) {
	roo1 := &TreeNode{
		Val: 1,
		Left: &TreeNode{
			Val: 3,
			Left: &TreeNode{
				Val: 5,
			},
		},
		Right: &TreeNode{
			Val: 2,
		},
	}
	roo2 := &TreeNode{
		Val: 2,
		Left: &TreeNode{
			Val: 1,
			Right: &TreeNode{
				Val: 4,
			},
		},
		Right: &TreeNode{
			Val: 3,
			Right: &TreeNode{
				Val: 7,
			},
		},
	}
	res := mergeTrees(roo1, roo2)
	roo3 := &TreeNode{
		Val: 3,
		Left: &TreeNode{
			Val: 4,
			Left: &TreeNode{
				Val: 5,
			},
			Right: &TreeNode{
				Val: 4,
			},
		},
		Right: &TreeNode{
			Val: 5,
			Right: &TreeNode{
				Val: 7,
			},
		},
	}

	assert.Equal(t, roo3.Val, res.Val)
	assert.Equal(t, roo3.Left.Val, res.Left.Val)
	assert.Equal(t, roo3.Right.Val, res.Right.Val)
	assert.Equal(t, roo3.Left.Left.Val, res.Left.Left.Val)
	assert.Equal(t, roo3.Left.Right.Val, res.Left.Right.Val)
	assert.Equal(t, roo3.Right.Right.Val, res.Right.Right.Val)
}
