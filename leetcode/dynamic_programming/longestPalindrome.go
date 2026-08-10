package dynamicprogramming

// 最长回文子串
// https://leetcode.cn/problems/longest-palindromic-substring/description/
func longestPalindrome(s string) string {
	if len(s) <= 1 {
		return s
	}
	expand := func(left, right int) int {
		for left >= 0 && right < len(s) && s[left] == s[right] {
			left--
			right++
		}
		return right - left - 1 // 回文长度
	}
	begin, maxLen := 0, 0
	for i := 0; i < len(s); i++ {
		l1 := expand(i, i)     // 奇数长度,中心 i
		l2 := expand(i, i+1)   // 偶数长度,中心 i,i+1 之间
		l := l1
		if l2 > l1 {
			l = l2
		}
		if l > maxLen {
			begin = i - (l-1)/2
			maxLen = l
		}
	}
	return s[begin : begin+maxLen]
}
