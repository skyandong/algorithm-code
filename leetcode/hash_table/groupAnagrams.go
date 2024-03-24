package hashtable

import (
	"sort"
)

// 字母异位词分组
// https://leetcode.cn/problems/group-anagrams/
func groupAnagrams(strs []string) [][]string {
	if len(strs) == 1 {
		return [][]string{{strs[0]}}
	}

	strMap := make(map[string][]string, len(strs))
	for _, str := range strs {
		byteSlice := []byte(str)
		sort.Slice(byteSlice, func(i, j int) bool {
			return byteSlice[i] < byteSlice[j]
		})
		sortedStr := string(byteSlice)
		strMap[sortedStr] = append(strMap[sortedStr], str)
	}

	res := make([][]string, 0, len(strMap))
	for _, strSlice := range strMap {
		res = append(res, strSlice)
	}
	return res
}

// func groupAnagrams(strs []string) [][]string {
// 	if len(strs) == 1 {
// 		return [][]string{{strs[0]}}
// 	}

// 	strMap := make(map[[26]byte][]string, len(strs))
// 	for _, str := range strs {
// 		alphabet := [26]byte{}
// 		for i := 0; i < len(str); i++ {
// 			alphabet[str[i]-'a']++
// 		}
// 		strMap[alphabet] = append(strMap[alphabet], str)
// 	}

// 	res := make([][]string, 0, len(strs))
// 	for _, strSlice := range strMap {
// 		res = append(res, strSlice)
// 	}
// 	return res
// }
