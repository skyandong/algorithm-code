package linkedlist

// 环形链表
// https://leetcode.cn/problems/linked-list-cycle/
func hasCycle(head *ListNode) bool {
	if head == nil || head.Next == nil {
		return false
	}
	for slow, quick := head, head.Next; quick != nil && quick.Next != nil; {
		if slow == quick {
			return true
		}
		slow = slow.Next
		quick = quick.Next.Next
	}
	return false
}
