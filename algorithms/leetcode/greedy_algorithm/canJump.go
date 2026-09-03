package greedyalgorithm

// 跳跃游戏
// https://leetcode.cn/problems/jump-game/
func canJump(nums []int) bool {
	for i, step := 0, nums[0]; i < len(nums) && step < len(nums)-1-i; i++ {
		if step == 0 && nums[i] == 0 {
			return false
		} else if step < nums[i] {
			step = nums[i]
		}
		step--
	}
	return true
}
