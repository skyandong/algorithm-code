package hashtable

// 最长连续序列
// https://leetcode.cn/problems/longest-consecutive-sequence
func longestConsecutive(nums []int) int {
	// 将所有元素存入一个集合中
	numSet := make(map[int]bool, len(nums))
	for _, num := range nums {
		numSet[num] = true
	}

	maxLen := 0
	// 遍历集合中的每个元素
	for num := range numSet {
		// 如果当前元素的前一个元素不存在于集合中，则当前元素是一个连续序列的起点
		if !numSet[num-1] {
			currentLen, currentNum := 1, num

			// 计算以当前元素为起点的连续序列的长度
			for numSet[currentNum+1] {
				currentNum++
				currentLen++
			}

			// 更新最大长度
			if currentLen > maxLen {
				maxLen = currentLen
			}
		}
	}
	return maxLen
}
