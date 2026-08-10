package linkedlist

func merge(head1, head2 *ListNode) *ListNode {
	dummyHead := &ListNode{}
	temp, temp1, temp2 := dummyHead, head1, head2
	for temp1 != nil && temp2 != nil {
		if temp1.Val <= temp2.Val {
			temp.Next = temp1
			temp1 = temp1.Next
		} else {
			temp.Next = temp2
			temp2 = temp2.Next
		}
		temp = temp.Next
	}
	if temp1 != nil {
		temp.Next = temp1
	} else if temp2 != nil {
		temp.Next = temp2
	}
	return dummyHead.Next
}

// 排序链表
// https://leetcode.cn/problems/sort-list/description/
func sortList(head *ListNode) *ListNode {
	if head == nil || head.Next == nil {
		return head
	}

	var count int
	for node := head; node != nil; count++ {
		node = node.Next
	}

	newHead := &ListNode{Next: head}
	for partLens := 1; partLens <= count; partLens <<= 1 {
		prev, cur := newHead, newHead.Next
		for cur != nil {
			head1 := cur
			for i := 1; i < partLens && cur.Next != nil; i++ {
				cur = cur.Next
			}

			head2 := cur.Next
			cur.Next = nil
			cur = head2
			for i := 1; i < partLens && cur != nil && cur.Next != nil; i++ {
				cur = cur.Next
			}

			var next *ListNode
			if cur != nil {
				next = cur.Next
				cur.Next = nil
			}

			prev.Next = merge(head1, head2)
			for prev.Next != nil {
				prev = prev.Next
			}

			cur = next
		}
	}
	return newHead.Next
}
