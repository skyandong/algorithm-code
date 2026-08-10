package dynamicprogramming

// 最长有效括号
// https://leetcode.cn/problems/longest-valid-parentheses/description/
func longestValidParentheses(s string) int {
	stack := []int{-1} // 栈顶始终是"有效子串起点的前一个位置";初始 -1 作基底
	maxLen := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '(' {
			stack = append(stack, i)
		} else {
			stack = stack[:len(stack)-1] // 先 pop
			if len(stack) == 0 {
				// 栈空:这个 ')' 未匹配,入栈当新基底
				stack = append(stack, i)
			} else {
				length := i - stack[len(stack)-1]
				if length > maxLen {
					maxLen = length
				}
			}
		}
	}
	return maxLen
}
