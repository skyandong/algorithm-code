package substring

// 滑动窗口最大值
// https://leetcode.cn/problems/sliding-window-maximum/
func maxSlidingWindow(nums []int, k int) []int {
	var ret []int
	dq := []int{} // 存索引,对应值从前到后单调递减;队首即窗口最大值
	for i := 0; i < len(nums); i++ {
		// 队首出窗:索引 <= i-k 的不在窗口 [i-k+1, i] 内
		for len(dq) > 0 && dq[0] <= i-k {
			dq = dq[1:]
		}
		// 维护单调递减:队尾比 nums[i] 小的永无出头之日,弹掉
		for len(dq) > 0 && nums[dq[len(dq)-1]] < nums[i] {
			dq = dq[:len(dq)-1]
		}
		dq = append(dq, i)
		// 窗口成型后(i >= k-1),队首即最大值
		if i >= k-1 {
			ret = append(ret, nums[dq[0]])
		}
	}
	return ret
}
