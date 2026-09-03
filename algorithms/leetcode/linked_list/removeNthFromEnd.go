package linkedlist

// 删除链表的倒数第 N 个结点
// https://leetcode.cn/problems/remove-nth-node-from-end-of-list/description/
func removeNthFromEnd(head *ListNode, n int) *ListNode {
	dummy := &ListNode{Next: head}
	fast, slow := dummy, dummy
	for i := 0; i < n; i++ { // 快指针先走 n 步,拉开 n+1 的间隔
		fast = fast.Next
	}
	for fast.Next != nil { // 同步走到快指针到末尾,slow.Next 即倒数第 n 个
		fast = fast.Next
		slow = slow.Next
	}
	slow.Next = slow.Next.Next
	return dummy.Next
}
