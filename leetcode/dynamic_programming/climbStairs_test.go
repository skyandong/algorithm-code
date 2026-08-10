package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClimbStairs(t *testing.T) {
	// LeetCode 示例
	assert.Equal(t, 2, climbStairs(2))
	assert.Equal(t, 3, climbStairs(3))

	// 边界
	assert.Equal(t, 1, climbStairs(1))

	// 递推
	assert.Equal(t, 5, climbStairs(4))
	assert.Equal(t, 8, climbStairs(5))
	assert.Equal(t, 13, climbStairs(6))

	// 较大
	assert.Equal(t, 89, climbStairs(10))

	// 上限 (LeetCode n<=45),验证不溢出
	assert.Equal(t, 1836311903, climbStairs(45))
}
