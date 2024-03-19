package arrays

func rotate(nums []int, k int) {
	l := len(nums)
	if k >= l {
		k = k % l
	}
	if k == 0 {
		return
	}
	copy(nums, append(nums[l-k:], nums[:l-k]...))
}
