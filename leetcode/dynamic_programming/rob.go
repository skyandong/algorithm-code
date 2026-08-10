package dynamicprogramming

// 打家劫舍
// https://leetcode.cn/problems/house-robber/description/
func rob(nums []int) int {
	prev, curr := 0, 0 // prev=dp[i-2], curr=dp[i-1]
	for _, x := range nums {
		// 抢这家(prev+x) 或 不抢这家(curr),取大
		prev, curr = curr, max(curr, prev+x)
	}
	return curr
}
