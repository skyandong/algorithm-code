package backtracking

// 电话号码的字母组合
// https://leetcode.cn/problems/letter-combinations-of-a-phone-number/description/
func letterCombinations(digits string) []string {
	if len(digits) == 0 {
		return nil
	}
	m := []string{"", "", "abc", "def", "ghi", "jkl", "mno", "pqrs", "tuv", "wxyz"}
	var res []string
	var dfs func(i int, path string)
	dfs = func(i int, path string) {
		if i == len(digits) {
			res = append(res, path)
			return
		}
		for _, c := range m[digits[i]-'0'] {
			dfs(i+1, path+string(c))
		}
	}
	dfs(0, "")
	return res
}
