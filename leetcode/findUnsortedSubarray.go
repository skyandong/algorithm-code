package leetcode

// findUnsortedSubarray 最短无序连续子数组
// https://leetcode.cn/problems/shortest-unsorted-continuous-subarray/
func findUnsortedSubarray(nums []int) int {
	begin, end := 0, -1
	max, min := nums[0], nums[len(nums)-1]
	for i := 0; i < len(nums); i++ {
		if nums[i] >= max {
			// 寻找有序数组右边界
			max = nums[i]
		} else {
			// 有序数组右边界最小值下标
			end = i
		}
		if nums[len(nums)-1-i] <= min {
			// 寻找有序数组左边界
			min = nums[len(nums)-1-i]
		} else {
			// 有序数组左边界最大值下标
			begin = len(nums) - 1 - i
		}
	}
	return end - begin + 1
}
