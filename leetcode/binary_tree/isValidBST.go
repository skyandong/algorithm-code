package binarytree

// 验证二叉搜索树
// https://leetcode.cn/problems/validate-binary-search-tree/description/
func isValidBST(root *TreeNode) bool {
	return isValid(root, nil, nil)
}

func isValid(root *TreeNode, left, right *int) bool {
	if root == nil {
		return true
	}

	if left != nil && root.Val <= *left {
		return false
	}
	if right != nil && root.Val >= *right {
		return false
	}
	return isValid(root.Left, left, &root.Val) && isValid(root.Right, &root.Val, right)
}

// func isValidBST(root *TreeNode) bool {
// 	stack := make([]*TreeNode, 0)
// 	var prev *int
//
// 	for root != nil || len(stack) > 0 {
// 		for root != nil {
// 			stack = append(stack, root)
// 			root = root.Left
// 		}
// 		root = stack[len(stack)-1]
// 		stack = stack[:len(stack)-1]
// 		if prev != nil && root.Val <= *prev {
// 			return false
// 		}
// 		prev = &root.Val
// 		root = root.Right
// 	}
// 	return true
// }
