package leetcode

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMoveZeroes(t *testing.T) {
	nums := []int{0, 1, 0, 3, 12}
	moveZeroes(nums)
	assert.Equal(t, []int{1, 3, 12, 0, 0}, nums)
}
