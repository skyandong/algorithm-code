package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRob(t *testing.T) {
	// LeetCode 示例 1: 抢 1+3=4
	assert.Equal(t, 4, rob([]int{1, 2, 3, 1}))

	// LeetCode 示例 2: 抢 2+9+1=12
	assert.Equal(t, 12, rob([]int{2, 7, 9, 3, 1}))

	// 空
	assert.Equal(t, 0, rob([]int{}))

	// 单元素
	assert.Equal(t, 5, rob([]int{5}))

	// 两元素:取较大
	assert.Equal(t, 2, rob([]int{1, 2}))
	assert.Equal(t, 2, rob([]int{2, 1}))

	// 首尾各取: [2,1,1,2] → 2+2=4
	assert.Equal(t, 4, rob([]int{2, 1, 1, 2}))

	// 大值交替: [100,1,1,100] → 100+100=200
	assert.Equal(t, 200, rob([]int{100, 1, 1, 100}))

	// 递增: 1+3+5=9
	assert.Equal(t, 9, rob([]int{1, 2, 3, 4, 5}))

	// 递减: 5+3+1=9
	assert.Equal(t, 9, rob([]int{5, 4, 3, 2, 1}))
}
