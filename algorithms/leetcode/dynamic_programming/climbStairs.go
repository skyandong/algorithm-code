package dynamicprogramming

// 爬楼梯
// https://leetcode.cn/problems/climbing-stairs/description/
func climbStairs(n int) int {
	a, b := 1, 1 // a=f(0), b=f(1)
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}
