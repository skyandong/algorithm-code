package dynamicprogramming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoinChange(t *testing.T) {
	// LeetCode 示例 1: 11 = 5+5+1,3 枚
	assert.Equal(t, 3, coinChange([]int{1, 2, 5}, 11))

	// LeetCode 示例 2: 只有面值 2,凑不出 3,返回 -1
	assert.Equal(t, -1, coinChange([]int{2}, 3))

	// LeetCode 示例 3: 金额 0,0 枚
	assert.Equal(t, 0, coinChange([]int{1}, 0))

	// 单枚硬币自凑
	assert.Equal(t, 1, coinChange([]int{5}, 5))

	// 凑不出:只有 2,凑 1
	assert.Equal(t, -1, coinChange([]int{2}, 1))

	// 贪心会错的例子:6 = 3+3 (2 枚),贪心先拿 4 会得 4+1+1 (3 枚)
	assert.Equal(t, 2, coinChange([]int{1, 3, 4}, 6))

	// 全用最大面值:100 = 20 个 5
	assert.Equal(t, 20, coinChange([]int{1, 2, 5}, 100))
}
