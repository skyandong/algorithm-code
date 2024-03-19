package binarysearch

func searchInsert(nums []int, target int) int {
	ret, left, right := -1, 0, len(nums)-1
	for mid := (left + right) >> 1; left < right; {
		if nums[mid] > target {
			right = mid - 1
		} else if nums[mid] < target {
			left = mid + 1
		} else {
			ret = mid
			break
		}
		mid = (left + right) >> 1
	}
	if ret < 0 {
		if nums[left] < target {
			ret = left + 1
		} else {
			ret = left
		}
	}
	return ret
}
