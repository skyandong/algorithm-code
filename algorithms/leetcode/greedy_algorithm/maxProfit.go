package greedyalgorithm

// 买卖股票的最佳时机
// https://leetcode.cn/problems/best-time-to-buy-and-sell-stock/
func maxProfit(prices []int) int {
	minPrice, maxProfits := prices[0], 0
	for i := 1; i < len(prices); i++ {
		maxProfits = max(maxProfits, prices[i]-minPrice)
		minPrice = min(minPrice, prices[i])
	}
	return maxProfits
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
