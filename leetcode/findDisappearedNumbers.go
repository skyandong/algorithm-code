package leetcode

// 找到所有数组中消失的数字
// https://leetcode.cn/problems/find-all-numbers-disappeared-in-an-array/
func findDisappearedNumbers(nums []int) []int {
	lens := len(nums)
	for _, v := range nums {
		if v = v % lens; v == 0 {
			v = lens
		}
		nums[v-1] = nums[v-1] + lens
	}
	var ret []int
	for k, v := range nums {
		if v <= lens {
			ret = append(ret, k+1)
		}
	}
	return ret
}
