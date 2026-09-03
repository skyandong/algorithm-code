package dynamicprogramming

import "math/big"

// 不同路径
// https://leetcode.cn/problems/unique-paths/description/

// uniquePaths 2D DP,O(mn) 时间 O(mn) 空间
func uniquePaths(m int, n int) int {
	dp := make([][]int, m)
	for i := range dp {
		dp[i] = make([]int, n)
		dp[i][0] = 1 // 第一列:只能一直往下
	}
	for j := 0; j < n; j++ {
		dp[0][j] = 1 // 第一行:只能一直往右
	}
	for i := 1; i < m; i++ {
		for j := 1; j < n; j++ {
			dp[i][j] = dp[i-1][j] + dp[i][j-1]
		}
	}
	return dp[m-1][n-1]
}

// uniquePathsRolling 滚动数组,O(mn) 时间 O(n) 空间
func uniquePathsRolling(m, n int) int {
	dp := make([]int, n)
	for i := range dp {
		dp[i] = 1
	}
	for i := 1; i < m; i++ {
		for j := 1; j < n; j++ {
			dp[j] += dp[j-1]
		}
	}
	return dp[n-1]
}

// uniquePathsMath 组合数 C(m+n-2, n-1),O(min(m,n)) 时间 O(1) 空间
func uniquePathsMath(m, n int) int {
	return int(new(big.Int).Binomial(int64(m+n-2), int64(n-1)).Int64())
}
