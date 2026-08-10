package linkedlist

// 两两交换链表中的节点
// https://leetcode.cn/problems/swap-nodes-in-pairs/description/
func swapPairs(head *ListNode) *ListNode {
	prevHead := &ListNode{
		Val:  0,
		Next: head,
	}
	temp := prevHead

	for temp.Next != nil && temp.Next.Next != nil {
		node1 := temp.Next
		node2 := temp.Next.Next

		temp.Next = node2
		node1.Next = node2.Next
		node2.Next = node1

		temp = node1
	}
	return prevHead.Next
}
