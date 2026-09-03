package graph_theory

// 腐烂的橘子
// https://leetcode.cn/problems/rotting-oranges/description/
func orangesRotting(grid [][]int) int {
	rotten := make(map[int][]int)
	rows, cols := len(grid), len(grid[0])
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			if grid[i][j] == 2 {
				rotten[i] = append(rotten[i], j)
			}
		}
	}

	var minutes int
	for {
		next := make(map[int][]int)
		for i, js := range rotten {
			for _, j := range js {
				infect(i-1, j, grid, next)
				infect(i+1, j, grid, next)
				infect(i, j-1, grid, next)
				infect(i, j+1, grid, next)
			}
		}
		if len(next) == 0 {
			break
		}
		minutes++
		rotten = next
	}

	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			if grid[i][j] == 1 {
				return -1
			}
		}
	}

	return minutes
}

func infect(i, j int, grid [][]int, next map[int][]int) {
	if i < 0 || j < 0 || i >= len(grid) || j >= len(grid[0]) {
		return
	}
	if grid[i][j] != 1 {
		return
	}
	grid[i][j] = 2
	next[i] = append(next[i], j)
}
