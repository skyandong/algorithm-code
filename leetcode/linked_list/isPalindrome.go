package linkedlist

// 回文链表
// https://leetcode.cn/problems/palindrome-linked-list/description/
func isPalindrome(head *ListNode) bool {
	var count int
	cur := head
	for cur != nil {
		cur = cur.Next
		count++
	}
	if count <= 1 {
		return true
	}

	newHead := head
	for mid := count / 2; mid > 0; mid-- {
		newHead = newHead.Next
	}

	var prev *ListNode
	for newHead != nil {
		next := newHead.Next
		newHead.Next = prev
		prev = newHead
		newHead = next
	}

	for mid := count / 2; mid > 0; mid-- {
		if head.Val != prev.Val {
			return false
		}
		prev = prev.Next
		head = head.Next
	}
	return true
}
