package binarysearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchMatrix(t *testing.T) {
	// LeetCode 示例
	m := [][]int{{1, 3, 5, 7}, {10, 11, 16, 20}, {23, 30, 34, 60}}
	assert.True(t, searchMatrix(m, 3))
	assert.False(t, searchMatrix(m, 13))

	// 单元素
	assert.True(t, searchMatrix([][]int{{5}}, 5))
	assert.False(t, searchMatrix([][]int{{5}}, 1))
	assert.False(t, searchMatrix([][]int{{5}}, 9))

	// 单行
	row := [][]int{{1, 2, 3, 4}}
	assert.True(t, searchMatrix(row, 4))
	assert.False(t, searchMatrix(row, 0))
	assert.False(t, searchMatrix(row, 5))

	// 单列
	col := [][]int{{1}, {3}, {5}}
	assert.True(t, searchMatrix(col, 3))
	assert.False(t, searchMatrix(col, 4))

	// 2x2
	m2 := [][]int{{1, 2}, {3, 4}}
	assert.True(t, searchMatrix(m2, 1))
	assert.True(t, searchMatrix(m2, 3))
	assert.True(t, searchMatrix(m2, 4))
	assert.False(t, searchMatrix(m2, 5))

	// 首/尾元素
	assert.True(t, searchMatrix(m, 1))
	assert.True(t, searchMatrix(m, 60))

	// 空矩阵
	assert.False(t, searchMatrix([][]int{}, 1))
	assert.False(t, searchMatrix([][]int{{}}, 1))
}
