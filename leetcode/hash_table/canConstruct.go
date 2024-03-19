package hashtable

// 赎金信，简单
// https://leetcode.cn/problems/ransom-note
func canConstruct(ransomNote string, magazine string) bool {
	lenR, lenM := len(ransomNote), len(magazine)
	if lenR > lenM {
		return false
	}

	myMap := make(map[int32]int)
	for _, val := range ransomNote {
		myMap[val]++
	}

	for _, val := range magazine {
		if myMap[val] > 0 {
			myMap[val]--
			lenR--
		}
	}
	return lenR == 0
}
