package leetcode

type ListNode struct {
	Val  int
	Next *ListNode
}

// AddTwoNumbers 两数相加
// https://leetcode-cn.com/problems/add-two-numbers/
func AddTwoNumbers(l1 *ListNode, l2 *ListNode) *ListNode {
	divisor := 0
	var newListHead, newListCur *ListNode
	for l1 != nil && l2 != nil {
		sum := l1.Val + l2.Val + divisor
		divisor = sum / 10
		if newListHead == nil {
			newListHead = &ListNode{Val: sum % 10}
			newListCur = newListHead
		} else {
			nextNode := &ListNode{Val: sum % 10}
			newListCur.Next = nextNode
			newListCur = newListCur.Next
		}
		l1 = l1.Next
		l2 = l2.Next
	}
	lastL := l1
	if lastL == nil {
		lastL = l2
	}
	for lastL != nil {
		sum := lastL.Val + divisor
		divisor = sum / 10
		nextNode := &ListNode{Val: sum % 10}
		newListCur.Next = nextNode
		newListCur = newListCur.Next
		lastL = lastL.Next
	}
	if divisor > 0 {
		newListCur.Next = &ListNode{Val: divisor}
	}
	return newListHead
}
