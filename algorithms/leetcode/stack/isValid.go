package leetcode

// 有效的括号
// https://leetcode.cn/problems/valid-parentheses/description/
func isValid(s string) bool {
	pairs := map[byte]byte{')': '(', ']': '[', '}': '{'}
	var stack []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if open, ok := pairs[c]; ok {
			// 闭括号:栈空或栈顶不匹配则非法,提前退出
			if len(stack) == 0 || stack[len(stack)-1] != open {
				return false
			}
			stack = stack[:len(stack)-1]
		} else {
			// 开括号:入栈
			stack = append(stack, c)
		}
	}
	return len(stack) == 0
}
