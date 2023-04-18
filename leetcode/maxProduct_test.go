package leetcode

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMaxProduct(t *testing.T) {
	assert.Equal(t, 6, maxProduct([]int{2, -3, -2, 4}))

	assert.Equal(t, 0, maxProduct([]int{-2, 0, -1}))
}
