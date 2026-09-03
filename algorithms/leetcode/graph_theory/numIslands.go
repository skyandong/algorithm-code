package graph_theory

// 岛屿数量
// https://leetcode.cn/problems/number-of-islands/description/
func numIslands(grid [][]byte) int {
	if len(grid) == 0 {
		return 0
	}
	rows, cols := len(grid), len(grid[0])
	var count int
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			if grid[i][j] == '1' {
				dfs(i, j, grid)
				count++
			}
		}
	}
	return count
}

func dfs(i, j int, grid [][]byte) {
	if i < 0 || j < 0 || i >= len(grid) || j >= len(grid[0]) {
		return
	}
	if grid[i][j] != '1' {
		return
	}
	grid[i][j] = '0'
	dfs(i-1, j, grid)
	dfs(i+1, j, grid)
	dfs(i, j-1, grid)
	dfs(i, j+1, grid)
}
