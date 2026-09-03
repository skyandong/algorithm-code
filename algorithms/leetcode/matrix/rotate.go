package matrix

// 旋转图像
// https://leetcode.cn/problems/rotate-image/description/
func rotate(matrix [][]int) {
	// matrix[i][j] → matrix[j][n-1-i]

	lens := len(matrix)
	for x := 0; x < lens/2; x++ {

		for y := x; y < lens-1-x; y++ {
			curx, cury := x, y
			num := matrix[curx][cury]
			for {
				nextx, nexty := cury, lens-1-curx

				temp := matrix[nextx][nexty]
				matrix[nextx][nexty] = num
				num = temp

				curx, cury = nextx, nexty

				if curx == x && cury == y {
					break
				}
			}
		}
	}
}
