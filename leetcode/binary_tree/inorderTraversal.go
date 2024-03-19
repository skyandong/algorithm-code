package binarytree

// 二叉树的中序遍历
// https://leetcode.cn/problems/binary-tree-inorder-traversal/
func inorderTraversal(root *TreeNode) []int {
	ret := make([]int, 0)
	inorderTraversalW(&ret, root)
	return ret
}

func inorderTraversalW(ret *[]int, root *TreeNode) {
	if root == nil {
		return
	}
	inorderTraversalW(ret, root.Left)
	*ret = append(*ret, root.Val)
	inorderTraversalW(ret, root.Right)
}
