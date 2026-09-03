package dualpointer

import "sort"

// 三数之和
// https://leetcode.cn/problems/3sum/description/
func threeSum(nums []int) [][]int {
	sort.Ints(nums)
	n := len(nums)
	var result [][]int
	if n < 3 || nums[0] > 0 || nums[n-1] < 0 {
		return result
	}
	for first := 0; first < n; first++ {
		if nums[first] > 0 {
			break
		}
		if first > 0 && nums[first] == nums[first-1] {
			continue
		}
		second, third := first+1, n-1
		for second < third {
			sum := nums[first] + nums[second] + nums[third]
			if sum == 0 {
				result = append(result, []int{nums[first], nums[second], nums[third]})
				second++
				third--
				for second < third && nums[second] == nums[second-1] {
					second++
				}
				for second < third && nums[third] == nums[third+1] {
					third--
				}
			} else if sum < 0 {
				second++
			} else {
				third--
			}
		}
	}
	return result
}
