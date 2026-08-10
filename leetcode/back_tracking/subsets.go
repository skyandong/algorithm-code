package backtracking

// 子集
// https://leetcode.cn/problems/subsets/description/
func subsets(nums []int) [][]int {
	var res [][]int
	backtrack(nums, []int{}, &res, 0)
	return res
}

func backtrack(nums, temp []int, res *[][]int, start int) {
	// 每次都将当前临时结果加入最终结果
	*res = append(*res, append([]int{}, temp...))

	// 从start位置开始遍历nums数组
	for i := start; i < len(nums); i++ {
		// 将当前元素加入临时结果
		temp = append(temp, nums[i])
		// 递归调用，从下一个位置开始继续生成子集
		backtrack(nums, temp, res, i+1)
		// 回溯，移除当前元素，以便尝试其他可能性
		temp = temp[:len(temp)-1]
	}
}
