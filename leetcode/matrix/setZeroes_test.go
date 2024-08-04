package matrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetZeroes(t *testing.T) {
	matrix := [][]int{{1, 1, 1}, {1, 0, 1}, {1, 1, 1}}
	setZeroes(matrix)
	assert.Equal(t, [][]int{{1, 0, 1}, {0, 0, 0}, {1, 0, 1}}, matrix)

	matrix = [][]int{{0, 1, 2, 0}, {3, 4, 5, 2}, {1, 3, 1, 5}}
	setZeroes(matrix)
	assert.Equal(t, [][]int{{0, 0, 0, 0}, {0, 4, 5, 0}, {0, 3, 1, 0}}, matrix)
}

func TestSetZeroes1(t *testing.T) {
	matrix := [][]int{{1, 1, 1}, {1, 0, 1}, {1, 1, 1}}
	setZeroes1(matrix)
	assert.Equal(t, [][]int{{1, 0, 1}, {0, 0, 0}, {1, 0, 1}}, matrix)

	matrix = [][]int{{0, 1, 2, 0}, {3, 4, 5, 2}, {1, 3, 1, 5}}
	setZeroes1(matrix)
	assert.Equal(t, [][]int{{0, 0, 0, 0}, {0, 4, 5, 0}, {0, 3, 1, 0}}, matrix)
}
