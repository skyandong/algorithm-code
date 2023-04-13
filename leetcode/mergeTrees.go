package leetcode

type TreeNode struct {
	Val   int
	Left  *TreeNode
	Right *TreeNode
}

// 合并二叉树
// https://leetcode.cn/problems/merge-two-binary-trees/
func mergeTrees(root1 *TreeNode, root2 *TreeNode) *TreeNode {
	if root1 == nil && root2 == nil {
		return nil
	}

	root3 := newTreeNode(root1, root2)
	work(root1, root2, root3)
	return root3
}

func work(root1 *TreeNode, root2 *TreeNode, root3 *TreeNode) {
	if root1 == nil && root2 == nil {
		return
	}

	var root1Left, root2Left *TreeNode
	if root1 != nil {
		root1Left = root1.Left
	}
	if root2 != nil {
		root2Left = root2.Left
	}
	if root1Left != nil || root2Left != nil {
		root3.Left = newTreeNode(root1Left, root2Left)
	}
	work(root1Left, root2Left, root3.Left)

	var root1Right, root2Right *TreeNode
	if root1 != nil {
		root1Right = root1.Right
	}
	if root2 != nil {
		root2Right = root2.Right
	}
	if root1Right != nil || root2Right != nil {
		root3.Right = newTreeNode(root1Right, root2Right)
	}
	work(root1Right, root2Right, root3.Right)
}

func newTreeNode(root1 *TreeNode, root2 *TreeNode) *TreeNode {
	var val int
	if root1 != nil {
		val += root1.Val
	}
	if root2 != nil {
		val += root2.Val
	}
	return &TreeNode{
		Val: val,
	}
}
