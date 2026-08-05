package graph_theory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrangesRotting(t *testing.T) {
	// LeetCode 示例 1: 3x3,4 分钟全部腐烂
	grid := [][]int{
		{2, 1, 1},
		{1, 1, 0},
		{0, 1, 1},
	}
	assert.Equal(t, 4, orangesRotting(grid))

	grid = [][]int{
		{2, 1, 0, 2},
	}
	assert.Equal(t, 1, orangesRotting(grid))

	// LeetCode 示例 2: 左下角鲜橘永远腐烂不到,返回 -1
	grid2 := [][]int{
		{2, 1, 1},
		{0, 1, 1},
		{1, 0, 1},
	}
	assert.Equal(t, -1, orangesRotting(grid2))

	// LeetCode 示例 3: 没有鲜橘,返回 0
	grid3 := [][]int{
		{0, 2},
	}
	assert.Equal(t, 0, orangesRotting(grid3))

	// 单个烂橘,无鲜橘
	assert.Equal(t, 0, orangesRotting([][]int{{2}}))

	// 单个鲜橘,无烂橘源头,返回 -1
	assert.Equal(t, -1, orangesRotting([][]int{{1}}))

	// 空格子
	assert.Equal(t, 0, orangesRotting([][]int{{0}}))

	// 全是鲜橘,无烂橘,返回 -1
	grid4 := [][]int{
		{1, 1},
		{1, 1},
	}
	assert.Equal(t, -1, orangesRotting(grid4))

	// 全是烂橘,返回 0
	grid5 := [][]int{
		{2, 2},
		{2, 2},
	}
	assert.Equal(t, 0, orangesRotting(grid5))

	// 单行多轮传播: 2 1 1 1 → 3 分钟
	assert.Equal(t, 3, orangesRotting([][]int{{2, 1, 1, 1}}))
}
