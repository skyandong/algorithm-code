package leetcode

// LengthOfLongestSubstring 无重复字符的最长子串
// https://leetcode-cn.com/problems/longest-substring-without-repeating-characters/
func LengthOfLongestSubstring(s string) int {
	mymap := make(map[uint8]int)
	var maxLen, begin, end int
	for ; end < len(s); end++ {
		if index, ok := mymap[s[end]]; ok && index >= begin {
			if max := end - begin; max > maxLen {
				maxLen = max
			}
			begin = index + 1
		}
		mymap[s[end]] = end
	}
	if end-begin > maxLen {
		maxLen = end - begin
	}
	return maxLen
}
