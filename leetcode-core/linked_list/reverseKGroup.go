package linkedlist

// K 个一组翻转链表
// https://leetcode.cn/problems/reverse-nodes-in-k-group/description/
//
// 题意: 给定链表头 head 和整数 k, 每 k 个节点为一组进行翻转, 不足 k 个的末尾保持原样。
// 例如 1->2->3->4->5, k=2 => 2->1->4->3->5。
//
// 解法选择理由:
//   - 本实现采用「先数长度, 再分组逐段翻转并重接」的方式, 而不是递归。
//   - 递归解法 (reverseKGroup(head.Next). ...) 写起来短, 但有调用栈开销 O(N/k),
//     且每层都要先走 k 步探路, 思路虽清晰但对"不足一组不翻"的终止条件容易写错。
//   - 本实现的思路: 第一遍 O(N) 数出总长 count, 算出需要翻 times = count/k 组,
//     之后每个完整组做一次局部翻转, 再把各段首尾接回去。
//   - 局部翻转复用了已有的 reverseList (迭代三指针), 每段翻转是 O(k), 共 times 段,
//     总翻转代价 O(times*k) = O(N); 加上数长度 O(N), 整体 O(N)。
//   - 空间 O(1) (reverseList 本身迭代, 不递归, 不开额外结构)。这是本题理论下界:
//     至少要把每个节点访问一遍, 无法做到 o(N)。
//
// 为什么不在翻转前先「切断再单独处理每段」, 而是每段都重新找起点?
//   - 这里 beginNode = 当前段在原链表中的起点, 通过外层循环推进 head 来定位段尾,
//     再把段尾.Next 置 nil 后交给 reverseList。这样 reverseList 拿到的是一条干净
//     的、以 nil 结尾的子链表, 不需要为 reverseList 做特殊改造。
//
// 易错点汇总 (面试官常追问):
//   1) times==0 必须提前返回, 否则后续 endNode.Next=head 会把原链表头接到自身形成环。
//   2) 段与段之间「尾接新头」: 翻转后原段头变成新段尾, 必须用它去接下一段, 而不是
//      用 subList (那是新段头)。
//   3) 末尾不足一组的剩余部分 (head != nil) 要原样接到最后一段的尾上, 不能漏, 否则
//      丢失尾部节点。
//   4) 第一组翻转后没有"上一段尾"可接, 需用 newHead==nil 判定首次, 单独记录新头。
func reverseKGroup(head *ListNode, k int) *ListNode {
	// 第一遍: 数总长 count。为什么不直接边走边翻? 因为需要知道是否凑得满一组,
	// 凑不满的尾部不能翻。先数长度是最直接、不易错的做法。
	var count int
	for cur := head; cur != nil; cur = cur.Next {
		count++
	}
	times := count / k
	if times == 0 {
		// 【关键坑】k > count 时一组都凑不满, 必须原样返回 head。
		// 若不提前返回, 下面 endNode 初值就是 head, 循环不执行,
		// 最后 endNode.Next=head 会把 head.Next 指回自己, 形成环且 newHead 为 nil。
		return head // 不足一组,原样返回(否则下面 endNode.Next=head 会成环)
	}

	var newHead *ListNode
	// endNode 始终指向「上一段翻转后的尾」。第一段翻转前还没有上一段,
	// 这里先初始化为原链表头 head: 因为第一段翻转后, 原头 head 恰好就是新尾,
	// 后面把它用来接第二段正好正确。先占位、第一轮里靠 continue 跳过对它的使用。
	endNode := head
	for i := 0; i < times; i++ {
		// 记录当前段在原链表中的起点; 翻转后它将成为本段的新尾。
		beginNode := head
		// 把 head 推到本段的最后一个节点 (向后走 k-1 步)。
		// 为什么是 k-1: 当前 head 已是段首, 走 k-1 步到达第 k 个节点即段尾。
		for j := 0; j < k-1; j++ {
			head = head.Next
		}

		// 暂存下一段的起点, 之后要把本段从原链表里「切断」。
		next := head.Next
		// 切断本段: 让本段以 nil 结尾, 这样 reverseList 拿到的是干净的子链表。
		// 【易错】若不置 nil, reverseList 会顺着 Next 一路把后续段也一起翻掉。
		head.Next = nil
		// head 前进到下一段起点, 供下一轮 i 使用 (也用于最后判断有没有剩余尾部)。
		head = next

		// 翻转本段, subList 是本段翻转后的新头。
		subList := reverseList(beginNode)
		if newHead == nil {
			// 第一段: 整条链的新头就是它, 此时还没有「上一段尾」需要接。
			// 注意这里 continue 很关键: 第一段翻转后尾就是 beginNode (原 head),
			// 而 endNode 初值正好是 head, 二者同一节点, 不必再赋值。
			newHead = subList
			continue
		}
		// 第二段及之后: 把上一段翻转后的尾 (endNode) 接到本段翻转后的头 (subList)。
		endNode.Next = subList
		// 更新 endNode 为本段翻转后的尾 —— 翻转后本段尾就是段首 beginNode。
		// 【易错】不要错写成 endNode = subList (那是新头不是新尾)。
		endNode = beginNode
	}
	// 处理不足一组的尾部: 若还有剩余节点 (head != nil), 原样接到最后一段尾后。
	// 【易错】整除时 head == nil, 这里 if 判断不能省 (否则 endNode.Next = nil 也无害,
	// 但写成无条件赋值会让人误以为一定有尾, 可读性差且易在改动时出错)。
	if head != nil {
		endNode.Next = head
	}
	return newHead
}
