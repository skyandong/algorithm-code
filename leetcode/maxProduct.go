package leetcode

// 乘积最大子数组
// https://leetcode.cn/problems/maximum-product-subarray/
func maxProduct(nums []int) int {
	maxNum, minNegative, indexNegative := 0, 0, -1
	for i := 0; i < len(nums); i++ {
		if nums[i] == 0 && indexNegative > 0 && minNegative/nums[indexNegative] > maxNum {

			maxNum = minNegative / nums[indexNegative]
		} else if nums[i] < 0 {
			if minNegative == 0 {
				indexNegative = i
				minNegative = nums[i]
			} else if minNegative < 0 {
				indexNegative = -1
				num := minNegative * nums[i]
				minNegative = 0
				if maxNum > 0 {
					maxNum *= num
				} else {
					maxNum = num
				}
			}
		} else {
			if maxNum == 0 {
				maxNum = nums[i]
			} else if minNegative < 0 {
				minNegative *= nums[i]
			} else {
				maxNum *= nums[i]
			}
		}
	}
	return maxNum
}
