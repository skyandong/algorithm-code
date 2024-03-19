package hashtable

// 最长连续序列
// https://leetcode.cn/problems/longest-consecutive-sequence
func longestConsecutive(nums []int) int {
	myMap := make(map[int]bool, len(nums))

	for i := range nums {
		myMap[nums[i]] = false
	}

	var maxLen int
	for key, val := range myMap {
		if val {
			continue
		}
		temp := 1
		for i := key - 1; ; i-- {
			if _, ok := myMap[i]; !ok {
				break
			}
			temp++
			myMap[i] = true
		}
		for i := key + 1; ; i++ {
			if _, ok := myMap[i]; !ok {
				break
			}
			temp++
			myMap[i] = true
		}
		if maxLen < temp {
			maxLen = temp
		}
	}

	return maxLen
}
