package backtracking

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCombinationSum(t *testing.T) {
	assert.Equal(t, [][]int{{2, 2, 2, 2}, {2, 3, 3}, {3, 5}}, combinationSum([]int{2, 3, 5}, 8))

	//combinationSum([]int{2, 3, 6, 7}, 7)
	//
	//combinationSum([]int{2}, 1)
	//a(1, []int{})
}
