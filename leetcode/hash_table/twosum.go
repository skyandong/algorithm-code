package hashtable

// TwoSum 两数之和
// https://leetcode-cn.com/problems/two-sum
func twoSum(nums []int, target int) []int {
	missingIndexMap := make(map[int]int)
	for key, num := range nums {
		if value, ok := missingIndexMap[num]; ok {
			return []int{value, key}
		}
		// 将缺失值映射到它们在切片中的索引位置
		missingIndexMap[target-num] = key
	}
	return []int{}
}
