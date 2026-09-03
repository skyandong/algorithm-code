package linkedlist

// 反转链表
// https://leetcode.cn/problems/reverse-linked-list/description/
//
// 题意：给定单链表头节点 head，返回反转后的链表头节点。
// 例如 1->2->3->4->5 反转后为 5->4->3->2->1。
//
// 算法选择理由（为什么用迭代三指针，而不选其他解法）：
//   - 解法一：迭代法（本实现）。用 prev/cur/next 三个指针原地翻转每个节点的 Next 指向。
//     优点：O(n) 时间、O(1) 额外空间，无递归栈开销，工程上最稳；不会因链表过长而栈溢出。
//   - 解法二：递归法。reverseList(head.Next) 后令 head.Next.Next = head 再置 head.Next = nil。
//     代码更短更"优雅"，但 O(n) 递归栈空间，链表超长（如 10^5）时可能爆栈；
//     且 head.Next.Next = head 这一步对新手反直觉，面试时容易写错指针方向。
//   - 本题数据规模通常不大，两种都能过；但面试官常追问"能否 O(1) 空间"，迭代法是更优答案。
//
// 复杂度：
//   - 时间 O(n)：遍历一次链表，每个节点处理一次，已是理论下界（必须访问每个节点才能翻转）。
//   - 空间 O(1)：仅用三个指针变量，与链表长度无关，已是该问题的空间下界。
//
// 易错点速览（面试常追问，见行内【】标注）：
//   1) prev 必须初始化为 nil，而不是 new(ListNode)；新链表尾部的 Next 应指向 nil。
//   2) 必须先用 next 暂存 head.Next，再改 head.Next，否则后续节点丢失。
//   3) 循环条件是 head != nil 而非 head.Next != nil，否则会漏掉最后一个节点的翻转。
//   4) 返回 prev 而不是 head：循环结束时 head == nil，prev 恰好指向原尾节点（即新头）。
func reverseList(head *ListNode) *ListNode {
	// prev 指向"已翻转部分的头"。
	// 【坑】初始值必须是 nil，不能是 new(ListNode)。因为原链表尾部（反转后的新头之前那个节点）
	// 的 Next 最终应指向 nil；若 prev 指向一个空节点，会多出一个 Val=0 的脏节点，破坏链表。
	var prev *ListNode

	// 用 head 本身作为游标遍历，不再额外声明 cur，节省一个变量；语义上 head 已不再是"原头"而是"当前节点"。
	// 【坑】循环条件必须是 head != nil，而不是 head.Next != nil。
	//   - 若写成 head.Next != nil：当 head 走到最后一个节点时其 Next==nil，循环提前退出，
	//     最后一个节点既没有被翻转 Next 指向 prev，prev 也未更新到最后一个节点，结果整个链表断在倒数第二。
	//   - 用 head != nil 时，最后一轮迭代会处理尾节点：把它的 Next 指向 prev，并把 prev 推进到尾节点。
	for head != nil {
		// 先暂存下一个节点。下一行就要改 head.Next，若不先存，后续链表就丢失了。
		// 【坑】顺序不能反：必须 "存 next -> 改 head.Next -> 移 prev -> 移 head"，
		//   若先改 head.Next 再取 next，head.Next 已被覆盖成 prev，next 就拿到错误的值。
		next := head.Next

		// 核心翻转动作：把当前节点的 Next 反过来指向 prev（已翻转部分的头）。
		head.Next = prev

		// prev 前进到当前节点：当前节点已成为"已翻转部分的新头"。
		prev = head
		// head 前进到原链表的下一个节点（之前暂存的 next），继续处理。
		head = next
	}
	// 【坑】返回 prev 而非 head。循环结束时 head == nil（已走到原链表尾部之后），
	// 而 prev 恰好指向原尾节点，即反转后的新头节点。返回 head 会得到 nil。
	return prev
}
