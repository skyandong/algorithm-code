package binarytree

var maxLens int

func diameter(tr *TreeNode) int {
	if tr == nil {
		return 0
	}

	leftDeep := diameter(tr.Left)
	rightDeep := diameter(tr.Right)
	maxLens = max(maxLens, leftDeep+rightDeep+1)
	return max(leftDeep, rightDeep) + 1
}

// 二叉树的直径
// https://leetcode.cn/problems/diameter-of-binary-tree/description/
func diameterOfBinaryTree(root *TreeNode) int {
	maxLens = 1
	diameter(root)
	return maxLens - 1
}
