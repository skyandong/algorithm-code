package arrays

// 最大子数组和
// https://leetcode.cn/problems/maximum-subarray/description/
func maxSubArray(nums []int) int {
	if len(nums) == 0 {
		return 0
	}
	cur, ans := nums[0], nums[0] // cur=以当前元素结尾的最大和,ans=全局最大
	for i := 1; i < len(nums); i++ {
		cur = max(cur+nums[i], nums[i]) // 延续前段 或 从 i 重新开始
		ans = max(ans, cur)
	}
	return ans
}
