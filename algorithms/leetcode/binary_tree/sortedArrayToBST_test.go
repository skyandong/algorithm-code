package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortedArrayToBST(t *testing.T) {
	// 正常用例
	root := sortedArrayToBST([]int{-10, -3, 0, 5, 9})
	assert.Equal(t, 0, root.Val)
	assert.Equal(t, -10, root.Left.Val)
	assert.Nil(t, root.Left.Left)
	assert.Equal(t, -3, root.Left.Right.Val)
	assert.Equal(t, 5, root.Right.Val)
	assert.Nil(t, root.Right.Left)
	assert.Equal(t, 9, root.Right.Right.Val)

	// 偶数个元素
	root2 := sortedArrayToBST([]int{0, 1, 2, 3, 4, 5})
	assert.Equal(t, 2, root2.Val)
	assert.Equal(t, 0, root2.Left.Val)
	assert.Nil(t, root2.Left.Left)
	assert.Equal(t, 1, root2.Left.Right.Val)
	assert.Equal(t, 4, root2.Right.Val)
	assert.Equal(t, 3, root2.Right.Left.Val)
	assert.Equal(t, 5, root2.Right.Right.Val)

	// 两个元素
	root3 := sortedArrayToBST([]int{1, 3})
	assert.Equal(t, 1, root3.Val)
	assert.Nil(t, root3.Left)
	assert.Equal(t, 3, root3.Right.Val)

	// 单个元素
	root4 := sortedArrayToBST([]int{1})
	assert.Equal(t, 1, root4.Val)
	assert.Nil(t, root4.Left)
	assert.Nil(t, root4.Right)

	// 空数组
	assert.Nil(t, sortedArrayToBST([]int{}))
}
