package graph_theory

// 腐烂的橘子
// https://leetcode.cn/problems/rotting-oranges/description/
func orangesRotting(grid [][]int) int {
	rows, cols := len(grid), len(grid[0])
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			if grid[i][j] == 2 {
				dfs(i, j, grid)
			}
		}
	}
}

func dfs(i, j int, grid [][]int) int {

}
