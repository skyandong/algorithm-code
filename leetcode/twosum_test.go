package leetcode

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTwoSum(t *testing.T) {
	nums := []int{2, 7, 11, 15}
	res := TwoSum(nums, 9)
	assert.Equal(t, []int{0, 1}, res)

	nums = []int{3, 2, 4}
	res = TwoSum(nums, 6)
	assert.Equal(t, []int{1, 2}, res)

	nums = []int{3, 3}
	res = TwoSum(nums, 6)
	assert.Equal(t, []int{0, 1}, res)
}
