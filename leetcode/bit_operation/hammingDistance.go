package bitoperation

import "math/bits"

// 汉明距离
// https://leetcode.cn/problems/hamming-distance/
func hammingDistance(x int, y int) int {
	return bits.OnesCount(uint(x ^ y))
}
