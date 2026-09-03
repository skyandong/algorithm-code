package heap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopKFrequent(t *testing.T) {
	// LeetCode 示例 1
	assert.ElementsMatch(t, []int{1, 2}, topKFrequent([]int{1, 1, 1, 2, 2, 3}, 2))

	// LeetCode 示例 2
	assert.Equal(t, []int{1}, topKFrequent([]int{1}, 1))

	// 全相同
	assert.Equal(t, []int{5}, topKFrequent([]int{5, 5, 5, 5}, 1))

	// 不同频次,top 2 明确
	assert.ElementsMatch(t, []int{2, 3}, topKFrequent([]int{1, 2, 2, 3, 3, 3}, 2))

	// 单一最高频
	assert.Equal(t, []int{3}, topKFrequent([]int{1, 2, 2, 3, 3, 3}, 1))

	// 含负数
	assert.ElementsMatch(t, []int{2, -1}, topKFrequent([]int{-1, -1, 2, 2, 2}, 2))

	// 两个并列最高 + 一个低频
	assert.ElementsMatch(t, []int{1, 2}, topKFrequent([]int{1, 1, 2, 2, 3}, 2))

	// k = 不同元素数
	assert.ElementsMatch(t, []int{1, 2, 3}, topKFrequent([]int{1, 1, 2, 2, 3, 3}, 3))
}
