package binarytree

func inorder(root *TreeNode, k *int, res *int) {
	if root == nil {
		return
	}
	inorder(root.Left, k, res)

	*k--
	if *k == 0 {
		*res = root.Val
		return
	}
	inorder(root.Right, k, res)
}

func kthSmallest(root *TreeNode, k int) int {
	result := new(int)
	inorder(root, &k, result)
	return *result
}