package arrays

func productExceptSelf(nums []int) []int {
	length := len(nums)

	// 创建结果数组，用于存储每个元素左侧的乘积
	result := make([]int, length)
	// 左侧乘积的初始值为1
	result[0] = 1
	for i := 1; i < length; i++ {
		// 计算当前元素左侧的乘积
		result[i] = result[i-1] * nums[i-1]
	}

	// 计算右侧乘积，并同时计算最终结果
	rightProduct := 1
	for i := length - 2; i >= 0; i-- {
		// 计算当前元素右侧的乘积
		rightProduct *= nums[i+1]
		// 将左侧乘积与右侧乘积相乘，得到最终结果
		result[i] *= rightProduct
	}
	return result
}
