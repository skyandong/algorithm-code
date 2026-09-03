package dualpointer

// 接雨水
// https://leetcode.cn/problems/trapping-rain-water/
//
// 题意:
//
//	给定 n 个非负整数表示每个柱子的高度,
//	求下雨后能接多少雨水。
//	例如 height = [0,1,0,2,1,0,1,3,2,1,2,1] 答案是 6。
//
// 算法选择理由（为什么选双指针而不是单调栈/动态规划）:
//
//	本题有三种主流解法, 面试时都能写出是加分项, 但双指针是"最优且最常被要求手写"的版本:
//
//	1) 动态规划 (DP): 预处理 leftMax[i] 和 rightMax[i] 两个数组,
//	   leftMax[i] = max(height[0..i]), rightMax[i] = max(height[i..n-1]),
//	   位置 i 能接的水 = min(leftMax[i], rightMax[i]) - height[i]。
//	   时间 O(n), 空间 O(n)。
//	   好处: 思路最直观, "木桶效应"一图就能讲清楚。
//	   不足: 需要两个额外数组, 空间没到下界。
//
//	2) 单调栈: 维护单调递减栈, 遇到更高柱子时弹出栈顶, 按"凹槽"逐层累加。
//	   时间 O(n), 空间 O(n)。
//	   好处: 思路是"横向分层"算水, 跟双指针/DP 的"竖向逐列"算水不同, 适合讲清两种视角。
//	   不足: 空间仍是 O(n), 且弹栈边界、左右柱距离容易写错。
//
//	3) 双指针 (本解法): 用 left/right 两个指针从两端向中间逼近,
//	   只维护两个标量 leftMax/rightMax (而非两个数组),
//	   每次移动较矮一侧的指针, 用该侧的 max 算出该列水量。
//	   时间 O(n), 空间 O(1)。
//	   ——空间 O(1) 是本题的理论下界, 因为输出只需一个累加器, 输入只读一次,
//	   不可能比常数额外空间更少; 时间 O(n) 也是下界, 因为每个柱子至少要被访问一次。
//
//	因此双指针是渐近最优解, 也是面试官最希望看到的写法。
//
// 双指针的核心思想（为什么每次移动较矮的一侧是正确的）:
//
//	位置 i 的水量 = min(左侧最高, 右侧最高) - height[i]。
//	关键观察: 我们不需要同时知道左右两侧精确的最高值, 只需要知道"较小的一侧"。
//	设当前 leftMax < rightMax, 那么无论 right 指针右边还有没有更高的柱子,
//	对于 left 位置而言, 它能装的水已经由 leftMax (较小侧) 唯一确定了 ——
//	因为右侧真实最高一定 >= rightMax > leftMax, min(左最高, 右最高) 必然等于 leftMax。
//	所以 left 位置的水量可以"立刻结算"并右移, 不必等待右侧信息全部到齐。
//	这就是双指针能省掉两个数组、降到 O(1) 空间的根本原因。
func trap(height []int) int {
	// ret 是答案累加器。题目保证高度非负, 接水量也非负, 初始 0 正确。
	var ret int

	n := len(height)
	// 边界: 少于 3 根柱子时不可能形成凹槽接水, 直接返回 0。
	// 【坑点】这一步不能省: n<=2 时下面 left<right 循环不会执行, 虽然不会越界,
	// 但显式提前返回能让边界意图清晰, 也方便面试时口头说明。
	if n < 3 {
		return 0
	}

	left, right := 0, n-1
	// leftMax: height[0..left] 中的最大值; rightMax: height[right..n-1 中的最大值。
	// 初始化为各自端点的高度。注意不是 0 ——若用 0 初始化, 在 height 全正的输入下
	// 首轮比较会有语义偏差(虽然这里靠 left<right 守卫住了一致性, 但语义上更易出错)。
	// 用端点高度初始化, 语义最干净: "已考察区间"初始就是单个端点。
	leftMax, rightMax := height[left], height[right]

	// 左右指针尚未相遇时, 每次只处理一侧并推进该侧指针。
	// 循环条件是 left < right 而非 left <= right:
	// 【坑点】两指针相遇的那一格不需要结算 ——相遇意味着它已是"当前考察区间的唯一高点",
	// 它本身不可能再装水(它是两侧 max 的来源之一, 高度等于自身, 水量恒为 0)。
	// 用 <= 也不会错(算出 0), 但 < 更贴合语义且少算一次无意义运算。
	for left < right {
		// 关键决策: 哪边 max 较小, 就处理并推进哪边。
		// WHY: 较小侧的水量已被较小 max 唯一锁定(见函数注释的论证), 可立即结算;
		// 而较大侧还可能遇到更高的柱子推高自己的 max, 此刻结算会少算。
		if leftMax < rightMax {
			// 左侧较矮, 处理 left 指针位置。
			// 先推进再更新 max: 这里先 left++ 把窗口右扩一格。
			left++

			// 更新左侧已见最高。新进入窗口的 height[left] 可能刷新 leftMax。
			// 【坑点】顺序: 必须先 left++ 再读 height[left] ——否则会重复处理 left 端点。
			// 这里用 if 而非 math.Max, 避免函数调用开销, 也更直白地表达"取较大"。
			if height[left] > leftMax {
				leftMax = height[left]
			}

			// 结算当前 left 位置的水量。
			// 由于 leftMax < rightMax (进入本分支的前提), 且右侧真实最高 >= rightMax,
			// 故 min(左最高, 右最高) = leftMax, 该列水量 = leftMax - height[left]。
			// leftMax 在上面已更新过: 若 height[left] 刚刷新了 leftMax,
			// 则 leftMax - height[left] = 0, 恰好表达"高点不接水", 逻辑自洽。
			ret += leftMax - height[left]
		} else {
			// 右侧较矮(或相等), 处理 right 指针位置。镜像对称于左侧分支。
			right--

			if height[right] > rightMax {
				rightMax = height[right]
			}

			ret += rightMax - height[right]
		}
	}

	return ret
}
