package binarysearch

// 搜索二维矩阵 (LeetCode 74: 每行升序,且每行首 > 上一行末,展平后整体有序)
// https://leetcode.cn/problems/search-a-2d-matrix/description/
func searchMatrix(matrix [][]int, target int) bool {
	m := len(matrix)
	if m == 0 {
		return false
	}
	n := len(matrix[0])
	if n == 0 {
		return false
	}
	lo, hi := 0, m*n-1
	for lo <= hi {
		mid := (lo + hi) / 2
		v := matrix[mid/n][mid%n]
		if v < target {
			lo = mid + 1
		} else if v > target {
			hi = mid - 1
		} else {
			return true
		}
	}
	return false
}
