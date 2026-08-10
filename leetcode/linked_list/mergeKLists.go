package linkedlist

// 合并 K 个升序链表
// https://leetcode.cn/problems/merge-k-sorted-lists/description/
func mergeKLists(lists []*ListNode) *ListNode {
	if len(lists) == 0 {
		return nil
	}
	return mergeRange(lists, 0, len(lists)-1)
}

// mergeRange 分治:把 lists[left..right] 两两归并;复用迭代版 mergeTwoLists 避免递归栈溢出
func mergeRange(lists []*ListNode, left, right int) *ListNode {
	if left == right {
		return lists[left]
	}
	mid := (left + right) >> 1
	return mergeTwoLists(mergeRange(lists, left, mid), mergeRange(lists, mid+1, right))
}
