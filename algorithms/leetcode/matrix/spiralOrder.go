package matrix

// 螺旋矩阵
// https://leetcode.cn/problems/spiral-matrix/description/
func spiralOrder(matrix [][]int) []int {
	rows, columns := len(matrix), len(matrix[0])

	left, right := 0, columns-1
	top, bottom := 0, rows-1

	res := make([]int, 0, right*bottom)

	for left <= right && top <= bottom {
		for col := left; col <= right; col++ {
			res = append(res, matrix[top][col])
		}
		top++

		for row := top; row <= bottom; row++ {
			res = append(res, matrix[row][right])
		}
		right--

		if left <= right && top <= bottom {
			for col := right; col >= left; col-- {
				res = append(res, matrix[bottom][col])
			}
			bottom--

			for row := bottom; row >= top; row-- {
				res = append(res, matrix[row][left])
			}
			left++
		}
	}

	return res
}
