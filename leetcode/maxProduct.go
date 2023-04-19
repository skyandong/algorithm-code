package leetcode

// 乘积最大子数组
// https://leetcode.cn/problems/maximum-product-subarray/
func maxProduct(nums []int) int {
	maxNum, temp := 0, 1
	firstNegativeNum, endNegativeNum := 0, 0
	for i := 0; i < len(nums); i++ {
		if nums[i] != 0 {
			if temp *= nums[i]; temp > maxNum {
				endNegativeNum = 0
				maxNum = temp
			}
		}
		if temp < 0 {
			endNegativeNum *= nums[i]
			if nums[i] < 0 {
				if firstNegativeNum == 0 {
					firstNegativeNum = temp
				} else {
					endNegativeNum = nums[i]
				}
			}
		}

		if (nums[i] == 0 || i == len(nums)-1) && temp < 0 {
			if endNegativeNum == 0 {
				if i > 1 && temp/firstNegativeNum > maxNum {
					maxNum = temp / firstNegativeNum
				}
			} else if firstNegativeNum > endNegativeNum {
				maxNum = temp / firstNegativeNum
			}
			temp, firstNegativeNum, endNegativeNum = 1, 0, 0
		}
	}
	return maxNum
}
