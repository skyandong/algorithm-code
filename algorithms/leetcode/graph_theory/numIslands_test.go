package graph_theory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNumIslands(t *testing.T) {
	// LeetCode 示例: 4x5 网格,1 个岛屿
	grid0 := [][]byte{
		{'1', '1', '1', '1', '0'},
		{'1', '1', '0', '1', '0'},
		{'1', '1', '0', '0', '0'},
		{'0', '0', '0', '0', '0'},
	}
	assert.Equal(t, 1, numIslands(grid0))

	// 单个岛屿
	grid := [][]byte{
		{'1', '1', '1', '0'},
		{'1', '0', '1', '0'},
		{'1', '1', '1', '0'},
		{'0', '0', '0', '0'},
	}
	assert.Equal(t, 1, numIslands(grid))

	// 多个岛屿
	grid2 := [][]byte{
		{'1', '1', '0', '0'},
		{'1', '1', '0', '0'},
		{'0', '0', '1', '1'},
		{'0', '0', '1', '1'},
	}
	assert.Equal(t, 2, numIslands(grid2))

	// 全是水
	grid3 := [][]byte{
		{'0', '0', '0'},
		{'0', '0', '0'},
		{'0', '0', '0'},
	}
	assert.Equal(t, 0, numIslands(grid3))

	// 全是陆地
	grid4 := [][]byte{
		{'1', '1', '1'},
		{'1', '1', '1'},
		{'1', '1', '1'},
	}
	assert.Equal(t, 1, numIslands(grid4))

	// 对角分布,4 邻接互不连通: 4 个孤岛
	grid5 := [][]byte{
		{'1', '0', '1'},
		{'0', '0', '0'},
		{'1', '0', '1'},
	}
	assert.Equal(t, 4, numIslands(grid5))

	// 单格陆地
	assert.Equal(t, 1, numIslands([][]byte{{'1'}}))

	// 单格水
	assert.Equal(t, 0, numIslands([][]byte{{'0'}}))
}
