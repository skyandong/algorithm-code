package matrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRotate(t *testing.T) {
	// LeetCode 示例: 3x3 顺时针 90°
	matrix := [][]int{
		{1, 2, 3},
		{4, 5, 6},
		{7, 8, 9},
	}
	rotate(matrix)
	assert.Equal(t, [][]int{
		{7, 4, 1},
		{8, 5, 2},
		{9, 6, 3},
	}, matrix)

	// 2x2
	matrix = [][]int{
		{1, 2},
		{3, 4},
	}
	rotate(matrix)
	assert.Equal(t, [][]int{
		{3, 1},
		{4, 2},
	}, matrix)

	// 4x4
	matrix = [][]int{
		{1, 2, 3, 4},
		{5, 6, 7, 8},
		{9, 10, 11, 12},
		{13, 14, 15, 16},
	}
	rotate(matrix)
	assert.Equal(t, [][]int{
		{13, 9, 5, 1},
		{14, 10, 6, 2},
		{15, 11, 7, 3},
		{16, 12, 8, 4},
	}, matrix)

	// 1x1: 单元素不变
	matrix = [][]int{{1}}
	rotate(matrix)
	assert.Equal(t, [][]int{{1}}, matrix)

	// 5x5: 奇数边长,中心元素不变
	matrix = [][]int{
		{1, 2, 3, 4, 5},
		{6, 7, 8, 9, 10},
		{11, 12, 13, 14, 15},
		{16, 17, 18, 19, 20},
		{21, 22, 23, 24, 25},
	}
	rotate(matrix)
	assert.Equal(t, [][]int{
		{21, 16, 11, 6, 1},
		{22, 17, 12, 7, 2},
		{23, 18, 13, 8, 3},
		{24, 19, 14, 9, 4},
		{25, 20, 15, 10, 5},
	}, matrix)
}
