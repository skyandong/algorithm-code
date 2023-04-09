package leetcode

// dailyTemperatures 每日温度
// https://leetcode.cn/problems/daily-temperatures
func dailyTemperatures(temperatures []int) []int {
	res := make([]int, len(temperatures))
	var stack []int
	for i := len(temperatures) - 1; i >= 0; i-- {
		for j := len(stack) - 1; j >= 0; j-- {
			if temperatures[i] < temperatures[stack[j]] {
				break
			}
			stack = stack[:len(stack)-1]
		}
		l := 0
		if len(stack) > 0 {
			l = stack[len(stack)-1] - i
		}
		res[i] = l
		stack = append(stack, i)
	}
	return res
}
