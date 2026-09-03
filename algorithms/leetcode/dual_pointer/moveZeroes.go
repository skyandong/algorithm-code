package dualpointer

// 移动零
// https://leetcode.cn/problems/move-zeroes/description/
func moveZeroes(nums []int) {
	left, right, size := 0, 0, len(nums)
	for right < size {
		if nums[right] != 0 {
			if left < right {
				nums[left], nums[right] = nums[right], nums[left]
			}
			left++
		}
		right++
	}
}
