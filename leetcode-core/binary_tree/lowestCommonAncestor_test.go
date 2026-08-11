package binarytree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLowestCommonAncestor(t *testing.T) {
	// 构造测试树:
	//        3
	//       / \
	//      5   1
	//     / \ / \
	//    6  2 0  8
	//      / \
	//     7   4
	n7 := &TreeNode{Val: 7}
	n4 := &TreeNode{Val: 4}
	n2 := &TreeNode{Val: 2, Left: n7, Right: n4}
	n6 := &TreeNode{Val: 6}
	n5 := &TreeNode{Val: 5, Left: n6, Right: n2}
	n0 := &TreeNode{Val: 0}
	n8 := &TreeNode{Val: 8}
	n1 := &TreeNode{Val: 1, Left: n0, Right: n8}
	root := &TreeNode{Val: 3, Left: n5, Right: n1}

	//// p、q 分布在根的两侧,LCA 为根节点: LCA(5,1)=3
	//assert.Equal(t, 3, lowestCommonAncestor(root, n5, n1).Val)

	//// p 是 q 的祖先,LCA 为 p 自身: LCA(5,4)=5
	//assert.Equal(t, 5, lowestCommonAncestor(root, n5, n4).Val)
	//
	//// 两个节点同在左子树: LCA(6,4)=5
	//assert.Equal(t, 5, lowestCommonAncestor(root, n6, n4).Val)
	//
	//// 两个节点同在右子树: LCA(0,8)=1
	//assert.Equal(t, 1, lowestCommonAncestor(root, n0, n8).Val)

	// 一深一浅分属两侧: LCA(7,8)=3
	assert.Equal(t, 3, lowestCommonAncestor(root, n7, n8).Val)

	// 根节点自身作为 p,q 在子树中: LCA(3,1)=3
	assert.Equal(t, 3, lowestCommonAncestor(root, root, n1).Val)
}
