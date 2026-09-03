package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSwapPairs(t *testing.T) {
	// LeetCode 示例
	assert.Equal(t, []int{2, 1, 4, 3},
		listVals(swapPairs(buildList(1, 2, 3, 4))))

	// 空
	assert.Nil(t, swapPairs(nil))

	// 单元素(奇数尾落单,不换)
	assert.Equal(t, []int{1}, listVals(swapPairs(buildList(1))))

	// 两元素
	assert.Equal(t, []int{2, 1}, listVals(swapPairs(buildList(1, 2))))

	// 三元素(最后一个落单)
	assert.Equal(t, []int{2, 1, 3}, listVals(swapPairs(buildList(1, 2, 3))))

	// 五元素
	assert.Equal(t, []int{2, 1, 4, 3, 5}, listVals(swapPairs(buildList(1, 2, 3, 4, 5))))

	// 六元素(偶数,全换)
	assert.Equal(t, []int{2, 1, 4, 3, 6, 5}, listVals(swapPairs(buildList(1, 2, 3, 4, 5, 6))))

	// 重复值
	assert.Equal(t, []int{1, 1}, listVals(swapPairs(buildList(1, 1))))

	// 负数
	assert.Equal(t, []int{-2, -1, -4, -3}, listVals(swapPairs(buildList(-1, -2, -3, -4))))
}
