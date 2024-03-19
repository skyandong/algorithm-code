package hashtable

import "sort"

// 字母异位词分组
// https://leetcode.cn/problems/group-anagrams/
func groupAnagrams(strs []string) [][]string {
	if len(strs) == 0 {
		return [][]string{{""}}

	}

	res := make([][]string, 0, len(strs))
	strMap := make(map[string]int, len(strs))
	for _, str := range strs {
		b := []byte(str)
		sort.Slice(b, func(i, j int) bool {
			return b[i] < b[j]
		})
		index, ok := strMap[string(b)]
		if !ok {
			res = append(res, []string{str})
			strMap[string(b)] = len(res) - 1
		} else {
			res[index] = append(res[index], str)
		}
	}
	return res
}
