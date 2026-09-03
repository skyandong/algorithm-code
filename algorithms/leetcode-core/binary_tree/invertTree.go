package binarytree

// 翻转二叉树
// https://leetcode.cn/problems/invert-binary-tree/description/
//
// 题意: 给定一棵二叉树根节点 root，将其每个节点的左右子树互换，返回根节点。
// 本质是对整棵树做一次"镜像"操作，树的结构发生改变（而非只是打印顺序）。
//
// 算法选择: 递归后序(DFS)。
//   - 为什么用后序而非前序? 翻转的核心动作是"交换当前节点的两个子树指针"。
//     无论前序还是后序都能得到正确结果，因为对每个节点而言，我们交换的是
//     子树指针本身(整棵子树整体搬过去)，并不依赖子树内部是否已被翻转。
//     这里写成的形式 root.Left, root.Right = invertTree(root.Right), invertTree(root.Left)
//     在赋值前会先求值右侧的两个递归调用，再一次性交换赋值，因此本行执行时
//     左右子指针尚未被改动，递归参数拿到的始终是"原始的"左右子树。
//   - 为什么不写两步赋值? 例如:
//         root.Left = invertTree(root.Right)
//         root.Right = invertTree(root.Left)   // 这里 root.Left 已被上一行覆盖!
//     第二行的 root.Left 不再是原始左子树，而是刚被翻转过的原右子树，
//     会导致结果错误。这是一个极其常见的坑，见下方【多行赋值顺序】说明。
//   - 为什么不用 BFS/迭代? 迭代写法(用队列层序遍历、逐节点交换左右指针)同样可行，
//     时间空间复杂度相同。递归写法最简洁、最不易出错，面试中默认先给递归解法，
//     被追问"能否避免递归/避免栈溢出"时再给迭代版本(用切片当队列即可)。
//
// 时间复杂度: O(n)，n 为节点数。每个节点恰好访问一次。
//   这已是理论下界: 要翻转整棵树，至少需要触碰每个节点一次，无法更快。
//
// 空间复杂度: O(h)，h 为树高，源于递归调用栈深度。
//   - 平衡树 h = O(log n)；退化为链表时 h = O(n)。
//   - 这不是"额外"空间下界(迭代写法用显式栈/队列同样 O(h))，而是该解法固有开销。
//   - 面试追问"最坏情况栈溢出怎么办": 可改用迭代(显式队列层序)规避递归栈过深。
func invertTree(root *TreeNode) *TreeNode {
	// 递归终止: 空节点没有子树可交换，直接返回 nil。
	// 易错点: 必须先判 nil 再访问 root.Left/Right，否则会 panic。
	// 这里返回 nil 而非 root 本身(root 此时就是 nil)，语义等价但更清晰。
	if root == nil {
		return nil
	}

	// 【多行赋值顺序——本题最大的坑】
	// 这里使用"多重赋值"(multiple assignment): Go 会先对右侧所有表达式
	// 求值(invertTree(root.Right) 和 invertTree(root.Left))，再统一赋值给左侧。
	// 因此即便左侧包含 root.Left，赋值发生时右侧的旧值早已计算完毕，
	// 不会出现"左指针被改写后再被右指针读取"的顺序错误。
	//
	// 对比错误写法(分两步，会错):
	//   root.Left = invertTree(root.Right)          // 左指针已被改成"翻转后的右子树"
	//   root.Right = invertTree(root.Left)          // 这里取到的 root.Left 是错的!
	//
	// 等价的另一种安全写法(先存临时变量，再交换):
	//   left, right := invertTree(root.Left), invertTree(root.Right)
	//   root.Left, root.Right = right, left
	// 两者结果一致，多重赋值版更简洁，面试时直接用即可。
	//
	// 顺序细节: 右侧两个递归的求值顺序在 Go 规范中是从左到右求值操作数，
	// 但因为本行右侧两个调用互不依赖(都读的是 root 的原始字段，且递归内部
	// 改的是各自的子树副本),所以求值顺序不影响最终结果。
	root.Left, root.Right = invertTree(root.Right), invertTree(root.Left)

	// 返回当前节点本身(指针不变，但其左右子树已完成翻转与交换)。
	// 注意: 翻转是"原地修改"树的结构，这里返回 root 是为了方便链式/递归调用，
	// 调用方拿到的仍是同一棵树的根，只是其结构已被镜像。
	return root
}
