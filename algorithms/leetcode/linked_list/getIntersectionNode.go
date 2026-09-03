package linkedlist

// 相交链表
// https://leetcode.cn/problems/intersection-of-two-linked-lists/description/
func getIntersectionNode(headA, headB *ListNode) *ListNode {
	if headA == nil || headB == nil {
		return nil
	}
	cur1, cur2 := headA, headB
	for cur1 != cur2 {
		// 各走一遍两条链表后总步数相同(a+b+c),必在交点相遇;无交点则同时到 nil
		if cur1 == nil {
			cur1 = headB
		} else {
			cur1 = cur1.Next
		}
		if cur2 == nil {
			cur2 = headA
		} else {
			cur2 = cur2.Next
		}
	}
	return cur1
}
