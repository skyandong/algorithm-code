package hashtable

// TwoSum 两数之和
// https://leetcode-cn.com/problems/two-sum
func twoSum(nums []int, target int) []int {
	// 新建 map 用于存储每个数值对应的索引位置
	numIndexMap := make(map[int]int, len(nums))
	for index, num := range nums {
		// 计算目标值与当前值的差值
		complement := target - num

		// 如果差值在 map 中存在，说明找到了两个数值的和为 target
		if complementIndex, ok := numIndexMap[complement]; ok {
			return []int{complementIndex, index}
		}

		// 将当前值和其下标存入 map
		numIndexMap[num] = index
	}
	return []int{}
}
