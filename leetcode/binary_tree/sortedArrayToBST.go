package binarytree

// 将有序数组转换为二叉搜索树
// https://leetcode.cn/problems/convert-sorted-array-to-binary-search-tree/description/
func build(head *TreeNode, left, right int, nums []int) *TreeNode {
	if left > right{
		return nil
	}
	mid := (left + right) / 2
	n := &TreeNode{
		Val: nums[mid],
	}
	if head == nil {
		head = n
	} else {
		if nums[mid] <= head.Val {
			head.Left = n
		} else {
			head.Right = n
		}
	}
	build(n, left, mid-1, nums)
	build(n, mid+1, right, nums)

	return head
}

func sortedArrayToBST(nums []int) *TreeNode {
	return build(nil, 0, len(nums)-1, nums)
}

// 优化代码
// func sortedArrayToBST(nums []int) *TreeNode {
// 	return buildBST(nums, 0, len(nums)-1)
// }

// func buildBST(nums []int, left, right int) *TreeNode {
// 	if left > right {
// 		return nil
// 	}
// 	mid := (left + right) / 2
// 	return &TreeNode{
// 		Val:   nums[mid],
// 		Left:  buildBST(nums, left, mid-1),
// 		Right: buildBST(nums, mid+1, right),
// 	}
// }


