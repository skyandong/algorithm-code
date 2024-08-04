package matrix

// 矩阵置零
// https://leetcode.cn/problems/set-matrix-zeroes
func setZeroes(matrix [][]int) {
	rowLen, colLen := len(matrix), len(matrix[0])

	col0Zero := false
	for _, r := range matrix {
		if r[0] == 0 {
			col0Zero = true
		}
		for i := 1; i < colLen; i++ {
			if r[i] == 0 {
				r[0] = 0
				matrix[0][i] = 0
			}
		}
	}

	for i := rowLen - 1; i >= 0; i-- {
		for j := 1; j < colLen; j++ {
			if matrix[i][0] == 0 || matrix[0][j] == 0 {
				matrix[i][j] = 0
			}
		}
		if col0Zero {
			matrix[i][0] = 0
		}
	}
}

func setZeroes1(matrix [][]int) {
	rowLen, colLen := len(matrix), len(matrix[0])

	row0Zero, col0Zero := false, false
	for i := 0; i < colLen; i++ {
		if matrix[0][i] == 0 {
			row0Zero = true
			break
		}
	}
	for i := 0; i < rowLen; i++ {
		if matrix[i][0] == 0 {
			col0Zero = true
			break
		}
	}

	for i := 1; i < rowLen; i++ {
		for j := 1; j < colLen; j++ {
			if matrix[i][j] == 0 {
				matrix[i][0] = 0
				matrix[0][j] = 0
			}
		}
	}

	for i := 1; i < rowLen; i++ {
		for j := 1; j < colLen; j++ {
			if matrix[i][0] == 0 || matrix[0][j] == 0 {
				matrix[i][j] = 0
			}
		}
	}

	if row0Zero {
		for i := 0; i < colLen; i++ {
			matrix[0][i] = 0
		}
	}

	if col0Zero {
		for i := 0; i < rowLen; i++ {
			matrix[i][0] = 0
		}
	}
}
