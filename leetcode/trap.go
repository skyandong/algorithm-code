package leetcode

// 接雨水
// https://leetcode.cn/problems/trapping-rain-water/
func trap(height []int) int {
	a := 0
	mi := 0
	s := 0
	for i := 0; i < len(height); i++ {
		if height[i] == 0 && a == 0 {
			continue
		}
		if height[i] > mi {
			mi = height[i]
			a = i
		}
		if height[i] < mi {
			s += mi - height[i]
		}
	}
	return s
}
