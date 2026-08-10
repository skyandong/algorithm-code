package linkedlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func listVals(head *ListNode) []int {
	var vals []int
	for head != nil {
		vals = append(vals, head.Val)
		head = head.Next
	}
	return vals
}

func TestReverseList(t *testing.T) {
	// 普通链表
	head := &ListNode{Val: 1, Next: &ListNode{Val: 2, Next: &ListNode{Val: 3, Next: &ListNode{Val: 4, Next: &ListNode{Val: 5}}}}}
	assert.Equal(t, []int{5, 4, 3, 2, 1}, listVals(reverseList(head)))

	// 单节点
	single := &ListNode{Val: 1}
	assert.Equal(t, []int{1}, listVals(reverseList(single)))

	// 两节点
	two := &ListNode{Val: 1, Next: &ListNode{Val: 2}}
	assert.Equal(t, []int{2, 1}, listVals(reverseList(two)))

	// 空链表
	assert.Nil(t, reverseList(nil))

	// 反转后头节点是原尾节点(指针相等);先存尾节点,reverseList 会原地改 head2
	head2 := &ListNode{Val: 1, Next: &ListNode{Val: 2, Next: &ListNode{Val: 3}}}
	tail2 := head2.Next.Next
	assert.Same(t, tail2, reverseList(head2))

	// 双反转回到原状:头指针不变
	head3 := &ListNode{Val: 1, Next: &ListNode{Val: 2, Next: &ListNode{Val: 3}}}
	assert.Same(t, head3, reverseList(reverseList(head3)))
}
