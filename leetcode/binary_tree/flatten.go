package binarytree

// 二叉树展开为链表
// https://leetcode.cn/problems/flatten-binary-tree-to-linked-list/description/
func flatten(root *TreeNode) {
	var prev *TreeNode
	var dfs func(*TreeNode)
	dfs = func(node *TreeNode) {
		if node == nil {
			return
		}
		dfs(node.Right)
		dfs(node.Left)
		node.Right = prev
		node.Left = nil
		prev = node
	}
	dfs(root)
}
