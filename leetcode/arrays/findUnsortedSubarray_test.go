package arrays

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindUnsortedSubarray(t *testing.T) {
	res := findUnsortedSubarray([]int{2, 6, 4, 8, 10, 9, 15})
	assert.Equal(t, 5, res)

	res = findUnsortedSubarray([]int{1, 2, 4, 5, 3})
	assert.Equal(t, 3, res)

	res = findUnsortedSubarray([]int{1, 2, 3, 4})
	assert.Equal(t, 0, res)

	res = findUnsortedSubarray([]int{1})
	assert.Equal(t, 0, res)

	res = findUnsortedSubarray([]int{1, 2, 3, 3, 3})
	assert.Equal(t, 0, res)

	res = findUnsortedSubarray([]int{1, 3, 2, 2, 2})
	assert.Equal(t, 4, res)
}
