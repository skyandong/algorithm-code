package arrays

// nextPermutation 将 nums 原地修改为字典序中的下一个排列。
// https://leetcode.cn/problems/next-permutation/description/
//
// 核心步骤：
//  1. 从右向左找到第一个 nums[i] < nums[i+1] 的位置 i；
//  2. 从右向左找到第一个大于 nums[i] 的元素并交换；
//  3. 反转 i+1 到末尾的后缀，使其变为最小的升序排列。
//
// 如果找不到 i，说明当前排列已经是最大排列，反转整个数组即可得到最小排列。
// 时间复杂度 O(n)，空间复杂度 O(1)。
func nextPermutation(nums []int) {
	i := len(nums) - 2

	// 后缀是非递增的，找到最右侧可以变大的位置。
	for i >= 0 && nums[i] >= nums[i+1] {
		i--
	}

	if i >= 0 {
		// 后缀非递增，从右侧找到的第一个更大元素就是最小的合法替换值。
		j := len(nums) - 1
		for nums[j] <= nums[i] {
			j--
		}
		nums[i], nums[j] = nums[j], nums[i]
	}

	// 交换后后缀仍是非递增的，反转后得到最小升序后缀。
	for left, right := i+1, len(nums)-1; left < right; left, right = left+1, right-1 {
		nums[left], nums[right] = nums[right], nums[left]
	}
}
