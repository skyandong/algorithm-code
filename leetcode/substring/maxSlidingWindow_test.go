package substring

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxSlidingWindow(t *testing.T) {
	// LeetCode 示例
	assert.Equal(t, []int{3, 3, 5, 5, 6, 7},
		maxSlidingWindow([]int{1, 3, -1, -3, 5, 3, 6, 7}, 3))

	// 递减序列:旧解法在这里翻车(给出 [5,2,2])
	assert.Equal(t, []int{5, 4, 3},
		maxSlidingWindow([]int{5, 4, 3, 2, 1}, 3))

	// k=1:每个元素自成窗口
	assert.Equal(t, []int{1, 3, -1},
		maxSlidingWindow([]int{1, 3, -1}, 1))

	// k=len:单个窗口,返回最大值
	assert.Equal(t, []int{3},
		maxSlidingWindow([]int{1, 3, -1}, 3))

	// 单元素
	assert.Equal(t, []int{5},
		maxSlidingWindow([]int{5}, 1))

	// 递增序列
	assert.Equal(t, []int{2, 3, 4},
		maxSlidingWindow([]int{1, 2, 3, 4}, 2))

	// 窗口内有重复最大值
	assert.Equal(t, []int{3, 3, 2},
		maxSlidingWindow([]int{1, 3, 1, 2, 0}, 3))

	// 含负数
	assert.Equal(t, []int{7, 4},
		maxSlidingWindow([]int{7, 2, 4}, 2))
}
