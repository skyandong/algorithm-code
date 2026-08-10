package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveNthFromEnd(t *testing.T) {
	// LeetCode 示例: [1,2,3,4,5], n=2 → [1,2,3,5]
	assert.Equal(t, []int{1, 2, 3, 5},
		listVals(removeNthFromEnd(buildList(1, 2, 3, 4, 5), 2)))

	// 删头节点(n=len)
	assert.Equal(t, []int{2, 3},
		listVals(removeNthFromEnd(buildList(1, 2, 3), 3)))

	// 删尾节点
	assert.Equal(t, []int{1, 2},
		listVals(removeNthFromEnd(buildList(1, 2, 3), 1)))

	// 单元素删完
	assert.Nil(t, removeNthFromEnd(buildList(1), 1))

	// 两元素
	assert.Equal(t, []int{2}, listVals(removeNthFromEnd(buildList(1, 2), 2)))
	assert.Equal(t, []int{1}, listVals(removeNthFromEnd(buildList(1, 2), 1)))

	// 长链删中间(倒数第 4)
	assert.Equal(t, []int{1, 2, 3, 5, 6, 7},
		listVals(removeNthFromEnd(buildList(1, 2, 3, 4, 5, 6, 7), 4)))

	// 长链删头(n=len)
	assert.Equal(t, []int{2, 3, 4},
		listVals(removeNthFromEnd(buildList(1, 2, 3, 4), 4)))
}
