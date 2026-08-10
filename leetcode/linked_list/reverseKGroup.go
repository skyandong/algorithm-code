package linkedlist

// K 个一组翻转链表
// https://leetcode.cn/problems/reverse-nodes-in-k-group/description/
func reverseKGroup(head *ListNode, k int) *ListNode {
	var count int
	for cur := head; cur != nil; cur = cur.Next {
		count++
	}
	times := count / k
	if times == 0 {
		return head // 不足一组,原样返回(否则下面 endNode.Next=head 会成环)
	}

	var newHead *ListNode
	endNode := head
	for i := 0; i < times; i++ {
		beginNode := head
		for j := 0; j < k-1; j++ {
			head = head.Next
		}

		next := head.Next
		head.Next = nil
		head = next

		subList := reverseList(beginNode)
		if newHead == nil {
			newHead = subList
			continue
		}
		endNode.Next = subList
		endNode = beginNode
	}
	if head != nil {
		endNode.Next = head
	}
	return newHead
}
