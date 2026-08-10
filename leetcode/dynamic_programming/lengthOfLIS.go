package dynamicprogramming

import "sort"

// 最长递增子序列
// https://leetcode.cn/problems/longest-increasing-subsequence/description/

// lengthOfLIS DP,O(n^2)
func lengthOfLIS(nums []int) int {
	if len(nums) == 0 {
		return 0
	}
	dp := make([]int, len(nums))
	for i := range dp {
		dp[i] = 1
	}
	ans := 1
	for i := 1; i < len(nums); i++ {
		for j := 0; j < i; j++ {
			if nums[j] < nums[i] && dp[j]+1 > dp[i] {
				dp[i] = dp[j] + 1
			}
		}
		if dp[i] > ans {
			ans = dp[i]
		}
	}
	return ans
}

// lengthOfLISNLogN 贪心+二分,O(n log n)
// tails[k] = 所有长度为 k+1 的递增子序列中,最小的末尾元素
func lengthOfLISNLogN(nums []int) int {
	tails := []int{}
	for _, num := range nums {
		pos := sort.Search(len(tails), func(i int) bool {
			return tails[i] >= num
		})
		if pos == len(tails) {
			tails = append(tails, num)
		} else {
			tails[pos] = num
		}
	}
	return len(tails)
}
