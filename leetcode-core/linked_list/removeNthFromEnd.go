package linkedlist

// 删除链表的倒数第 N 个结点
// https://leetcode.cn/problems/remove-nth-node-from-end-of-list/description/
//
// 【题意】给定单链表头节点 head 与整数 n，删除倒数第 n 个节点并返回新头。
// 题目保证 n 合法（1 <= n <= 链表长度），所以这里无需校验越界。
//
// 【算法选择：快慢双指针 + dummy 哨兵节点】
// 直觉做法是先遍历一遍求长度 L，再正向走到第 L-n 个前驱节点删除——需要两趟扫描。
// 双指针解法用"间隔"替代"长度计数"，一趟扫描即可定位前驱，是面试期望的最优解。
//
// 核心思路（为什么是"快指针先走 n 步"而不是 n+1 步）：
//   - 目标是让 slow 停在"待删节点的前驱"上，因为单链表删除必须持有前驱指针。
//   - 若 fast 先走 n 步，则 fast 与 slow 之间恒保持 n 的间隔（指针数）。
//     当 fast.Next == nil 即 fast 抵达尾节点时，fast 与 slow 之间隔着 n 条 Next 边，
//     即 slow.Next 就是倒数第 n 个节点（slow 是其前驱）。这正是删除所需的位置。
//   - 若误写成 fast 先走 n+1 步，slow 会停在待删节点本身而非前驱，
//     此时无法删除（单链表无法回退），这是最常见的 off-by-one 坑。
//   - 也可理解为：删除倒数第 n 个，等价于保留正数第 (L-n) 个之前的全部，
//     slow 需停在正数第 (L-n-1) 个；fast 走 n 步恰好让 slow 比 fast 落后 n。
//
// 【为什么必须用 dummy 哨兵节点】
//   - 当要删的是头节点（n == 链表长度）时，head 自身就是被删对象，没有"前驱"。
//     不用 dummy 的话需要单独写一段 if 分支处理删头，代码丑且易漏。
//   - dummy.Next = head 让头节点也有了一个统一的前驱，删头与删中间走同一套逻辑，
//     且最终统一返回 dummy.Next（即便头被删，dummy.Next 也指向新头）。
//   - 这是链表题的通用范式：凡涉及"可能修改头"的操作，先建 dummy，可消除所有特判。
//
// 【复杂度】
//   - 时间 O(L)：快指针共走 n + (L-n) = L 步，一趟遍历。这已是单链表的理论下界，
//     因为定位倒数第 n 个至少要看到尾节点才能知道总长度的"倒数"含义。
//   - 空间 O(1)：只用常数个指针。dummy 节点是栈上一次分配，不随链长增长。
//
// 【面试官常追问】
//   1. n 不合法（n > 链表长度）怎么办？——本题保证合法；若不保证，需在第一趟
//      "fast 先走 n 步"时检查 fast 是否提前变 nil，提前 return head 即可。
//   2. 能否用递归？——可以，回程时计数，O(L) 栈空间，不如双指针优。一般不采用。
//   3. 删除后链表节点的内存：Go 有 GC，无需手动 free；C++ 需 delete 防泄漏。
func removeNthFromEnd(head *ListNode, n int) *ListNode {
	// 建立哨兵节点。注意 Next 指向 head，使 head 也拥有统一前驱；
	// 这样删头节点（n==len）与删中间节点走完全相同的代码路径，免去特判。
	dummy := &ListNode{Next: head}
	// 快慢指针同起点（都从 dummy 出发），靠"快指针先走"来制造间隔。
	// 起点选 dummy 而非 head 至关重要：保证 slow 最终落在待删节点的前驱，
	// 而非待删节点本身——单链表删除必须持有前驱。
	fast, slow := dummy, dummy

	// 快指针先走 n 步，制造"fast 比 slow 领先 n 条边"的间隔。
	// 走 n 步而非 n+1 步是关键（见上方注释的 off-by-one 分析）。
	// 这里不检查 fast==nil，因为题目保证 n <= 链表长度，fast 必然不会越界。
	for i := 0; i < n; i++ { // 快指针先走 n 步,拉开 n+1 的间隔
		fast = fast.Next
	}

	// 快慢同步推进，直到 fast 抵达尾节点（fast.Next == nil）。
	// 此时 slow 落后 fast 恰好 n 条边，即 slow.Next 就是倒数第 n 个节点。
	// 注意循环条件是 fast.Next != nil 而非 fast != nil：
	//   - 若写成 fast != nil，fast 会越过尾节点走到 nil，slow 多走一步，
	//     slow 停在待删节点本身而非前驱，删除就会出错。这是第二个易错点。
	for fast.Next != nil { // 同步走到快指针到末尾,slow.Next 即倒数第 n 个
		fast = fast.Next
		slow = slow.Next
	}

	// 此时 slow 是待删节点的前驱，直接改 Next 跳过待删节点即完成删除。
	// slow.Next 必非 nil（因为 n>=1 且 fast 已到尾，slow.Next 一定存在）。
	// 这里不需要保存被删节点单独释放——Go GC 会回收。
	slow.Next = slow.Next.Next

	// 返回 dummy.Next 而非 head：若头节点被删，head 已不在链中，
	// dummy.Next 才是真实新头。这是用 dummy 范式时的统一收尾。
	return dummy.Next
}
