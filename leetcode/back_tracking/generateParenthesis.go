package backtracking

// 括号生成
// https://leetcode.cn/problems/generate-parentheses/description/
func generateParenthesis(n int) []string {
	res := []string{}
	backtrackParen(n, []byte{}, 0, 0, &res)
	return res
}

func backtrackParen(n int, path []byte, open, close int, res *[]string) {
	if len(path) == 2*n {
		*res = append(*res, string(path))
		return
	}

	if open < n {
		path = append(path, '(')
		backtrackParen(n, path, open+1, close, res)
		path = path[:len(path)-1]
	}

	if close < open {
		path = append(path, ')')
		backtrackParen(n, path, open, close+1, res)
		path = path[:len(path)-1]
	}
}
