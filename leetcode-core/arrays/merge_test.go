package arrays

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	num1 := []int{1, 2, 3, 0, 0, 0}
	merge(num1, 3, []int{2, 5, 6}, 3)
	assert.Equal(t, []int{1, 2, 2, 3, 5, 6}, num1)

	num1 = []int{1}
	merge(num1, 1, []int{}, 0)
	assert.Equal(t, []int{1}, num1)

	num1 = []int{0}
	merge(num1, 0, []int{1}, 1)
	assert.Equal(t, []int{1}, num1)

	num1 = []int{1, 2, 3, 0, 0, 0}
	merge(num1, 3, []int{2, 5, 6}, 3)
	assert.Equal(t, []int{1, 2, 2, 3, 5, 6}, num1)

	num1 = []int{1}
	merge(num1, 1, []int{}, 0)
	assert.Equal(t, []int{1}, num1)

	num1 = []int{0}
	merge(num1, 0, []int{1}, 1)
	assert.Equal(t, []int{1}, num1)
}
