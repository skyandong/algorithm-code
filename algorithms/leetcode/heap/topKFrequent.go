package heap

// 前 K 个高频元素
// https://leetcode.cn/problems/top-k-frequent-elements/description/
func topKFrequent(nums []int, k int) []int {
	// 1. 统计频次
	count := map[int]int{}
	for _, n := range nums {
		count[n]++
	}
	// 2. 桶:bucket[freq] = 该频次的元素列表(频次范围 [1, n])
	buckets := make([][]int, len(nums)+1)
	for n, c := range count {
		buckets[c] = append(buckets[c], n)
	}
	// 3. 从高频往低频收集 k 个
	var res []int
	for freq := len(buckets) - 1; freq >= 0 && len(res) < k; freq-- {
		for _, n := range buckets[freq] {
			res = append(res, n)
			if len(res) == k {
				return res
			}
		}
	}
	return res
}
