package linkedlist

// 环形链表
// https://leetcode.cn/problems/linked-list-cycle/
//
// 【题意】给定单链表头节点 head，判断链表中是否存在环。
// 环即：某个节点的 Next 指针指向链表中在它之前的节点，使得遍历会无限循环下去。
// 注意本题只要返回 bool，不需要返回环的入口（那是 142 题）。
//
// 【算法选择：Floyd 快慢指针】
// 可选解法各有取舍：
//   1. 哈希集合：遍历把每个 *ListNode 存进 map，遇到重复即有环。
//      时间 O(n)、空间 O(n)。写法最直白，但面试官会追问"能不能 O(1) 空间"。
//   2. Floyd 龟兔赛跑（快慢双指针）：slow 每次走 1 步，quick 每次走 2 步。
//      若有环，quick 终会从背后追上 slow（在环内相遇）；若无环，quick 先到 nil。
//      时间 O(n)、空间 O(1)。这是面试标准答案，也是本题最优解。
//   3. 标记法（修改节点/给节点加 visited 字段）：破坏原链表结构，不可取。
//
// 这里采用解法 2。空间 O(1) 是该问题的理论下界——在不修改输入的前提下，
// 必须用某种"游标"在链表上游走，至少常数个指针。
//
// 【时间复杂度严格分析】（面试常被追问）
//   - 无环时：quick 走到尾，O(n)。
//   - 有环时：设链表非环部分长 a，环长 b。slow 进入环时，quick 已在环内，
//     两者距离差 d 满足 0<=d<b。每一步 quick 相对 slow 靠近 1 步（因为快 1 步/步），
//     所以最多再走 b-1 步必然相遇。总步数 O(a+b)=O(n)。
//   - 关键不变量：quick - slow 的速度差是常数 1，因此在环内必相遇（不会"跳过"）。
//     这是"快 2 慢 1"而非"快 3 慢 1"的原因——后者在 b 为偶数时可能永不相遇（步差 2 与环长不互质时跳过）。
func hasCycle(head *ListNode) bool {
	// 【边界】空链表或只有一个节点（且无自环）必然无环。
	//   - head == nil：空链表。
	//   - head.Next == nil：单节点，唯一的 Next 指向 nil，构不成环。
	//   注意：本题节点 Val 不代表"是否自环"，单节点不可能成环（没有第二个节点可指回）。
	//   这个特判同时也保证了下面 quick 初值 head.Next 不会在空节点上解引用。
	if head == nil || head.Next == nil {
		return false
	}

	// 【初始化：错开一步】slow 起点为 head，quick 起点为 head.Next。
	//   为什么要错开？如果 slow 和 quick 都从 head 出发，循环第一轮判断 slow==quick
	//   立刻为 true，会把"无环的起点相遇"误判成有环。
	//   错开 1 步等价于"已经各自走了一步再开始比较"，规避了起点的假相遇。
	//   【易错点】很多写法把循环写成 for slow != quick，并把初始化和步进写在循环体里，
	//   那种写法若初始 slow==quick 会有同样问题；这里用"先错开再判相等"是更稳妥的模式。
	//
	// 【循环条件：quick != nil && quick.Next != nil】
	//   因为 quick 每次走 2 步（quick.Next.Next），所以必须同时保证 quick 本身和 quick.Next 都不为 nil，
	//   否则 quick.Next.Next 会 panic（空指针解引用）。
	//   【坑】只写 quick != nil 是不够的：若 quick.Next == nil，quick.Next.Next 直接 panic。
	//   注意是判断 quick 这一侧（快指针），不需要判 slow——slow 一步走 1，只要 quick 还在走，slow 必然安全。
	for slow, quick := head, head.Next; quick != nil && quick.Next != nil; {
		// 每轮"先判相等，再走步"。相遇即有环，立即返回。
		// 注意比较的是指针（节点地址），不是 Val——Val 重复不等于环（环的判据是 Next 形成回路）。
		if slow == quick {
			return true
		}
		slow = slow.Next      // 慢指针走 1 步
		quick = quick.Next.Next // 快指针走 2 步
	}
	// quick 走到 nil（或 quick.Next 为 nil），说明链表有终点，无环。
	return false
}
