package linkedlist

// 反转链表
// https://leetcode.cn/problems/reverse-linked-list/description/
func reverseList(head *ListNode) *ListNode {
	var prev *ListNode
	for head != nil {
		next := head.Next

		head.Next = prev

		prev = head
		head = next
	}
	return prev
}
