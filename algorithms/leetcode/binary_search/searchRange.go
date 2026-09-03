package binarysearch

// 在排序数组中查找元素的第一个和最后一个位置
// https://leetcode.cn/problems/find-first-and-last-position-of-element-in-sorted-array/
func searchRange(nums []int, target int) []int {
	leftIndex := binarySearch(nums, target, true)
	rightIndex := binarySearch(nums, target, false) - 1
	if leftIndex <= rightIndex && rightIndex < len(nums) &&
		nums[leftIndex] == target && nums[rightIndex] == target {
		return []int{leftIndex, rightIndex}
	}
	return []int{-1, -1}
}

func binarySearch(nums []int, target int, lower bool) int {
	left, right, ans := 0, len(nums)-1, len(nums)
	for left <= right {
		mid := (left + right) / 2
		if nums[mid] > target || (lower && nums[mid] >= target) {
			right = mid - 1
			ans = mid
		} else {
			left = mid + 1
		}
	}
	return ans
}
