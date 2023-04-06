package leetcode

// TwoSum 两数之和
// https://leetcode-cn.com/problems/two-sum
func TwoSum(nums []int, target int) (res []int) {
	mymap := make(map[int]int)
	for index1 := 0; index1 < len(nums); index1++ {
		if index2, ok := mymap[nums[index1]]; ok {
			res = append(res, index2, index1)
			break
		}
		mymap[target-nums[index1]] = index1
	}
	return res
}
