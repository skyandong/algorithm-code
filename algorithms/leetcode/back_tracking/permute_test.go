package backtracking

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPermute(t *testing.T) {
	result := permute([]int{1, 2, 3})
	assert.Len(t, result, 6)
	assert.Contains(t, result, []int{1, 2, 3})
	assert.Contains(t, result, []int{1, 3, 2})
	assert.Contains(t, result, []int{2, 1, 3})
	assert.Contains(t, result, []int{2, 3, 1})
	assert.Contains(t, result, []int{3, 1, 2})
	assert.Contains(t, result, []int{3, 2, 1})

	assert.Equal(t, [][]int{{0}}, permute([]int{0}))
}
