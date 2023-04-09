package leetcode

func cycleMeet(head *ListNode) *ListNode {
	if head == nil || head.Next == nil {
		return nil
	}
	for slow, quick := head, head.Next; quick != nil && quick.Next != nil; {
		if slow == quick {
			return slow
		}
		slow = slow.Next
		quick = quick.Next.Next
	}
	return nil
}

// detectCycle 环形链表 II
// https://leetcode.cn/problems/linked-list-cycle-ii/
func detectCycle(head *ListNode) *ListNode {
	meet := cycleMeet(head)
	if meet == nil {
		return nil
	}
	for begin, meet := head, meet.Next; ; meet = meet.Next {
		if begin == meet {
			return begin
		}
		begin = begin.Next
	}
}
