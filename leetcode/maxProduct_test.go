package leetcode

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMaxProduct(t *testing.T) {
	assert.Equal(t, 48, maxProduct([]int{2, -3, -2, 4}))

	assert.Equal(t, 7000, maxProduct([]int{1, -3, 4, -5, 5, -7, 10}))

	assert.Equal(t, 0, maxProduct([]int{-2, 0, -1}))
}
