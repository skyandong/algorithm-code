package dynamicprogramming

// 零钱兑换
// https://leetcode.cn/problems/coin-change/description/
func coinChange(coins []int, amount int) int {
	dp := make([]int, amount+1)
	for i := range dp {
		dp[i] = amount + 1 // 哨兵:比任何合法解都大;用 amount+1 而非 MaxInt 避免 dp[i-c]+1 溢出
	}
	dp[0] = 0

	for i := 1; i <= amount; i++ {
		for _, c := range coins {
			if c <= i && dp[i-c]+1 < dp[i] {
				dp[i] = dp[i-c] + 1
			}
		}
	}

	if dp[amount] > amount {
		return -1
	}
	return dp[amount]
}
