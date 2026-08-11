package binarytree

// 二叉树的层序遍历
// https://leetcode.cn/problems/binary-tree-level-order-traversal/description/
//
// 题意: 给定二叉树根节点, 自顶向下、逐层从左到右返回节点值, 每一层是一个 []int,
//       最终返回 [][]int。空树返回空切片(nil)。
//
// 算法选择: BFS(广度优先搜索) + 滚动双层队列。
//
// 为什么是 BFS 而不是 DFS?
//   层序天然就是"按距离根的层数"分组, BFS 用队列保证"先入先出 + 同层先于下一层",
//   正好对应层序的访问顺序。DFS 也能做(递归时把深度 depth 当作 res 的下标往对应层
//   追加), 但需要额外的 depth 参数, 且依赖递归栈; BFS 更直观, 也是面试默认写法。
//
// 为什么用"两个队列交替"而不是"单队列 + size 计数"?
//   本文件用 nextQueue 收集下一层节点, 内层 for 遍历完当前 queue 后整体替换:
//       queue = nextQueue
//   这是 Go 里最自然的写法, 因为 Go 没有现成的"队列首指针前移"的轻量写法
//   (slice 的底层数组不会被复用, 但本写法每层分配新 slice, 可读性最好)。
//   另一种等价写法是"单队列 + 每层开始时记录 size := len(queue), 再 for j<size",
//   不显式建 nextQueue, 直接把子节点 push 进原队列末尾。两者正确性、复杂度一致,
//   本写法把"当前层 / 下一层"在数据结构上彻底分开, 思维负担更小, 不容易写错边界。
//
// 时间复杂度: O(n), 每个节点恰好入队出队各一次。这是理论下界, 因为每个节点都
//   必须被访问才能输出它的值, 不可能低于 O(n)。
// 空间复杂度: O(n), 队列最大宽度出现在"最宽的一层", 最坏(满二叉树底层)约 n/2,
//   量级 O(n)。这是 BFS 的固有代价, 无法避免(DFS 递归栈是 O(h), 与宽度无关,
//   这是 BFS vs DFS 在空间上的核心权衡, 面试官常追问"哪个省空间")。
func levelOrder(root *TreeNode) (res [][]int) {
	// 【坑】空树必须单独处理。否则 queue 初始化为 []*TreeNode{nil},
	// 第一层内层 for 会执行一次, 对 nil.Val 取值直接 panic。
	// 这里直接 return, 利用命名返回值 res 的零值 nil, 与 LeetCode 期望的空结果一致。
	if root == nil {
		return
	}

	// 初始队列只放根节点。注意是切片字面量语法 []*TreeNode{root}, 不要写成
	// make 后再 append, 这里字面量更简洁且等价。
	queue := []*TreeNode{root}

	// i 既充当外层循环计数(语义上是"当前层号, 0-indexed"), 又被用作 res 的下标。
	// 用 i < len(queue) 的写法在这里没有意义(因为 i 永远远小于 queue 长度),
	// 真正的循环条件是 len(queue) > 0: 只要当前层还有节点就继续。
	// 这里的 for i 是"两件合一": i 计数 + len(queue)>0 作终止判断。
	for i := 0; len(queue) > 0; i++ {
		// 先为本层预分配一个空切片并 append 到 res。
		// 必须先 append 占位, 否则后面 res[i] 会越界 panic ——
		// 因为 i 这一层还不在 res 里。这是"先占坑再填值"的典型模式。
		res = append(res, []int{})

		// nextQueue 收集下一层的所有节点。每层新建, 干净利落,
		// 避免和当前 queue 混在一起导致"分不清谁属于哪一层"。
		var nextQueue []*TreeNode

		// 【关键边界】内层 for 的上界是 len(queue), 即"当前层的节点数"。
		// 这是本写法正确的核心: 在进入内层循环前快照当前层大小, 之后即便
		// 往 nextQueue 里 append 也只影响下一层, 不会污染当前层迭代。
		// 若误写成动态判断(比如边 push 边遍历同一队列且用 len(queue) 做上界),
		// 就会把下一层节点也并进当前层, 导致层划分错误 —— 这是层序遍历最常见的 bug。
		for j := 0; j < len(queue); j++ {
			node := queue[j]

			// 用 res[i] 直接定位到本层的切片再 append 值。
			// 注意 res[i] 本身是个 slice header, append 会自动处理扩容并返回新 header,
			// 必须写回 res[i] = append(res[i], ...), 不能只写 append 不接收 ——
			// 否则底层数组虽然变了, 但 res[i] 这个 header 没更新长度, 值会丢失。
			// 这里的 i 在外层循环里已固定, 安全。
			res[i] = append(res[i], node.Val)

			// 子节点按"左先右后"入队, 保证同层节点在下一层的相对顺序是从左到右。
			// 【易错】顺序写反(先 Right 后 Left)不会影响"同层值的顺序"(因为同层是
			// 按入队顺序出队的), 但会导致下一层整体左右颠倒。务必保持 Left 在前。
			if node.Left != nil {
				nextQueue = append(nextQueue, node.Left)
			}
			if node.Right != nil {
				nextQueue = append(nextQueue, node.Right)
			}
		}

		// 整体替换: 用下一层队列覆盖当前队列, 进入下一轮外层循环。
		// 当 nextQueue 为空(即最后一层都是叶子, 没有子节点)时,
		// queue 变空, 外层 len(queue) > 0 为 false, 循环自然终止。
		queue = nextQueue
	}
	return
}
