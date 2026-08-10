package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetIntersectionNode(t *testing.T) {
	// LeetCode 示例: listA [4,1,8,4,5], listB [5,6,1,8,4,5], 在 8 处相交
	shared := &ListNode{Val: 8, Next: &ListNode{Val: 4, Next: &ListNode{Val: 5}}}
	headA := &ListNode{Val: 4, Next: &ListNode{Val: 1, Next: shared}}
	headB := &ListNode{Val: 5, Next: &ListNode{Val: 6, Next: &ListNode{Val: 1, Next: shared}}}
	assert.Same(t, shared, getIntersectionNode(headA, headB))

	// 相交点就是 listA 的头(listB 直接接到 listA 头)
	shared2 := &ListNode{Val: 1, Next: &ListNode{Val: 2, Next: &ListNode{Val: 3}}}
	assert.Same(t, shared2, getIntersectionNode(shared2, &ListNode{Val: 4, Next: shared2}))

	// 两个头是同一个节点
	same := &ListNode{Val: 7}
	assert.Same(t, same, getIntersectionNode(same, same))

	// 单节点相交
	one := &ListNode{Val: 9}
	assert.Same(t, one, getIntersectionNode(one, one))

	// 长度不同仍相交
	shared3 := &ListNode{Val: 6, Next: &ListNode{Val: 7}}
	headA3 := &ListNode{Val: 1, Next: &ListNode{Val: 2, Next: shared3}}
	headB3 := &ListNode{Val: 9, Next: shared3}
	assert.Same(t, shared3, getIntersectionNode(headA3, headB3))

	// 不相交
	assert.Nil(t, getIntersectionNode(
		&ListNode{Val: 1, Next: &ListNode{Val: 2}},
		&ListNode{Val: 3, Next: &ListNode{Val: 4}}))

	// 一边为空
	assert.Nil(t, getIntersectionNode(nil, &ListNode{Val: 1}))

	// 两边都空
	assert.Nil(t, getIntersectionNode(nil, nil))
}
