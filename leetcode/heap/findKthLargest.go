package heap

func partSort(array []int, left int, right int) (begin int) {
	begin = left
	key := array[right]
	for end := right; begin < end; {
		for begin < end && array[begin] >= key {
			begin++
		}
		for begin < end && array[end] <= key {
			end--
		}
		if begin < end {
			array[begin], array[end] = array[end], array[begin]
		}
	}
	array[begin], array[right] = array[right], array[begin]
	return
}

func sort(array []int, begin, end, k int) {
	if begin >= end {
		return
	}
	dev := partSort(array, begin, end)
	if dev == k {
		return
	}
	if dev > k {
		sort(array, begin, dev-1, k)
	} else {
		sort(array, dev+1, end, k)
	}
}

// 数组中的第K个最大元素
// https://leetcode.cn/problems/kth-largest-element-in-an-array/description/
func findKthLargest(nums []int, k int) int {
	sort(nums, 0, len(nums)-1, k-1)
	return nums[k-1]
}
