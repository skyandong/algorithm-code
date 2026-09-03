package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func buildList(vals ...int) *ListNode {
	dummy := &ListNode{}
	cur := dummy
	for _, v := range vals {
		cur.Next = &ListNode{Val: v}
		cur = cur.Next
	}
	return dummy.Next
}

func TestIsPalindrome(t *testing.T) {
	// 回文(偶数)
	assert.True(t, isPalindrome(buildList(1, 2, 2, 1)))

	// 回文(奇数)
	assert.True(t, isPalindrome(buildList(1, 2, 3, 2, 1)))

	// 单节点
	assert.True(t, isPalindrome(buildList(1)))

	// 空链表
	assert.True(t, isPalindrome(nil))

	// 两节点
	assert.True(t, isPalindrome(buildList(1, 1)))
	assert.False(t, isPalindrome(buildList(1, 2)))

	// 三节点
	assert.True(t, isPalindrome(buildList(1, 2, 1)))
	assert.False(t, isPalindrome(buildList(1, 2, 3)))

	// 较长回文
	assert.True(t, isPalindrome(buildList(1, 2, 3, 3, 2, 1)))
	assert.True(t, isPalindrome(buildList(1, 2, 3, 4, 3, 2, 1)))

	// 较长非回文
	assert.False(t, isPalindrome(buildList(1, 2, 4, 3, 2, 1)))
	assert.False(t, isPalindrome(buildList(1, 2, 3, 4, 3, 2, 1, 0)))

	// 含负数
	assert.True(t, isPalindrome(buildList(-1, 2, -1)))
	assert.False(t, isPalindrome(buildList(-1, 2, 1)))
}
