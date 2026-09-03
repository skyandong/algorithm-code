package backtracking

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSubsets(t *testing.T) {
	assert.Equal(t, [][]int{{}, {1}, {1, 2}, {1, 2, 3}, {1, 3}, {2}, {2, 3}, {3}},
		subsets([]int{1, 2, 3}))

	assert.Equal(t, [][]int{{}, {0}}, subsets([]int{0}))
}
