package dualpointer

// 盛最多水的容器
// https://leetcode.cn/problems/container-with-most-water/description/
func maxArea(height []int) int {
	l, r := 0, len(height)-1
	ans := 0
	for l < r {
		area := min(height[l], height[r]) * (r - l)
		ans = max(ans, area)
		// 移动较矮的一端:移动高的一端不可能让面积变大(宽度变小,高度仍被矮边封顶)
		if height[l] < height[r] {
			l++
		} else {
			r--
		}
	}
	return ans
}
