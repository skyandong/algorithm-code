package substring

// 和为 K 的子数组
// https://leetcode.cn/problems/subarray-sum-equals-k/
func subarraySum(nums []int, k int) int {
	ret, num, lenN := 0, 0, len(nums)
	mymap := make(map[int]int, lenN)
	for left := 0; left < lenN; left++ {
		mymap[num]++
		num += nums[left]
		ret += mymap[num-k]
	}
	return ret
}
