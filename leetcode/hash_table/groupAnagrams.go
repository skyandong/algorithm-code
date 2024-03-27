package hashtable

import (
	"sort"
)

// 字母异位词分组
// https://leetcode.cn/problems/group-anagrams/
func groupAnagrams(strs []string) [][]string {
	if len(strs) == 1 {
		// 只有一个字符串时，直接返回
		return [][]string{{strs[0]}}
	}

	// 新建映射，以字母计数作为键，对应的字符串切片作为值
	anagramMap := make(map[[26]int][]string, len(strs))
	for _, str := range strs {
		// 新建表示 26 个英文字母计数的数组
		letterCount := [26]int{}
		for _, ch := range str {
			letterCount[ch-'a']++
		}
		// 将字符串添加到映射中相应的字母计数键下
		anagramMap[letterCount] = append(anagramMap[letterCount], str)
	}

	res := make([][]string, 0, len(strs))
	for _, strSlice := range anagramMap {
		// 遍历映射，将字符串切片添加到结果集中
		res = append(res, strSlice)
	}
	return res
}

func groupAnagrams_sort(strs []string) [][]string {
	if len(strs) == 1 {
		return [][]string{{strs[0]}}
	}

	// key 为字典排序后的字符串，val 为字符串列表
	anagramMap := make(map[string][]string, len(strs))
	for _, str := range strs {
		// 排序字符串
		byteSlice := []byte(str)
		sort.Slice(byteSlice, func(i, j int) bool {
			return byteSlice[i] < byteSlice[j]
		})
		sortedStr := string(byteSlice)
		// 插入字符串
		anagramMap[sortedStr] = append(anagramMap[sortedStr], str)
	}

	res := make([][]string, 0, len(anagramMap))
	for _, strSlice := range anagramMap {
		res = append(res, strSlice)
	}
	return res
}
