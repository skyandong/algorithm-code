package leetcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDailyTemperatures(t *testing.T) {
	res := dailyTemperatures([]int{73, 74, 75, 71, 69, 72, 76, 73})
	assert.Equal(t, []int{1, 1, 4, 2, 1, 1, 0, 0}, res)
}
