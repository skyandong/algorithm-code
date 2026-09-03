package dualpointer

// 接雨水
// https://leetcode.cn/problems/trapping-rain-water/
func trap(height []int) int {
	var ret int
	lensHeight := len(height)
	stack := make([]int, 0, lensHeight)
	for i := lensHeight - 1; i >= 0; i-- {
		for j := len(stack) - 1; j >= 0; j-- {
			if height[i] > height[stack[j]] {
				var num, hahaha = 1, height[i] - height[stack[j]]

				if j > 0 && stack[j-1]-stack[j] > 1 {
					num = stack[j-1] - stack[j]
				}
				ret += (height[i] - height[stack[j]]) * num

				if j == 0 {
					ret -= hahaha * (stack[j] - i)
				}

				stack = stack[:j]
			}
		}
		stack = append(stack, i)
	}
	return ret
}
