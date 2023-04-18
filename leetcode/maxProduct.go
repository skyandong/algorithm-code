package leetcode

// 乘积最大子数组
// https://leetcode.cn/problems/maximum-product-subarray/
func maxProduct(nums []int) int {
	maxNum, minNegative, indexNegative := 0, 0, -1
	temp := 1
	for i := 0; i < len(nums); i++ {
		if temp *= nums[i]; temp > maxNum {
			maxNum = temp
		}
		if nums[i] < 0 && indexNegative = -1 {

		}
		if nums[i] == 0 {
			if maxNum > 0 {
				// 啥也不用做哦
			}
			// 寻找左数组乘积最大数组
			if temp < 0 {
				// 要看负数数量是否 > 1 呢
				// 是：   比较 [0:firt] 的乘积和[end:0]的乘积谁更小
				// 否：   比较 [0:first] 的乘积 [first + 1:0]的乘积谁更大
			}

			// 将前后 index 为 -1
		}
	}
	return maxNum
}
