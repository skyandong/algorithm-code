package hashtable

// TwoSum 两数之和
// https://leetcode-cn.com/problems/two-sum
//
// 题意: 在整数数组 nums 中找到两个下标不同的元素，使它们的和等于 target，
//   返回这两个下标。题目保证恰好存在一组解，且同一元素不能重复使用两次。
//
// 解法选择理由(为什么选哈希一次遍历，而非别的解法):
//  1. 暴力双循环 O(n^2): 简单但太慢，n=1e4 时就 1e8 量级，面试只能作为引子提一句。
//  2. 排序 + 双指针 O(n log n): 看似更优，但排序会破坏原始下标，需要额外存一份索引，
//     且相等情况的去重更绕。对本题"返回下标"的诉求来说，反而更复杂、更易写错。
//  3. 哈希表两次遍历 O(n): 第一遍建 map，第二遍查 complement。能过，但要特判"同一元素
//     用两次"的情况(如 nums=[3,4], target=6 时 3+3=6 但只用了一个 3)。
//  4. 哈希表一次遍历 O(n): 边遍历边查，map 里只存"已经访问过的元素"。这样天然规避了
//     "同一元素用两次"的问题——因为查 complement 时，当前元素还没进 map，不可能命中自己。
//     既是最优时间，又是最简洁的写法，是面试标准答案。
//
// 复杂度:
//   时间 O(n): 每个元素最多一次 map 读写，均摊 O(1)。
//   空间 O(n): 最坏情况下 map 存满 n-1 个元素(解在末尾时)。
//   时间复杂度已是理论下界——必须至少看一遍数组才能确定解，Ω(n) 无法再降。
func twoSum(nums []int, target int) []int {
	// 预分配容量 len(nums)：避免 map 扩容时的 rehash 开销。
	// 注意 cap 给到 len(nums) 略有冗余(解可能不需要存全部)，但避免了渐进扩容的多次拷贝，
	// 在面试和实际工程里都是更稳的写法。
	numIndexMap := make(map[int]int, len(nums))

	for index, num := range nums {
		// complement = target - num，即"要找的另一个数"。
		// 为什么用减法而不是遍历去找两数之和: 一次减法把"找配对"从 O(n) 降为 O(1) 的 map 查找，
		// 这是把整体复杂度从 O(n^2) 降到 O(n) 的关键。
		complement := target - num

		// 在"已访问过的元素"里查 complement。
		// 【关键坑点: 查找与写入的顺序】必须先查再写，不能先写再查。
		//   反例: 若先 numIndexMap[num] = index 再查 complement，当 num == complement 时
		//   (如 nums=[3,3], target=6 的第一个 3)，会把自己当成配对命中，返回 [0,0] 这种非法结果。
		//   先查再写则保证 map 里只有"过去的元素"，绝不会命中当前元素自身。
		// 【返回顺序坑点】返回的是 {complementIndex, index}，即"先出现的下标在前"。
		//   因为 complementIndex 一定来自更早的位置(它已经在 map 里了)，所以 complementIndex < index，
		//   符合题目要求的"下标顺序"惯例。若反过来写 {index, complementIndex} 用例就会挂。
		if complementIndex, ok := numIndexMap[complement]; ok {
			// 找到配对，直接返回。题目保证恰好一组解，无需继续遍历。
			return []int{complementIndex, index}
		}

		// 当前元素没命中任何配对，把它存入 map 供后续元素查找。
		// 【重复元素坑点】若数组中有重复值(如两个 3)，后一个 3 会覆盖前一个 3 的索引。
		//   但这不影响正确性: 因为"恰好一组解"，如果两个重复值都是解的一部分，那么在访问到第二个时，
		//   第一个已经在 map 里，会直接命中并返回(走的是上面 if 分支)，根本到不了这里的覆盖。
		//   覆盖只发生在"被覆盖的值不是解"的情况下，所以无副作用。
		numIndexMap[num] = index
	}

	// 题目保证有解，理论上不会走到这里。返回空切片是为了让函数有确定的返回值，避免编译器/调用方
	// 对"无返回"的恐慌；也便于万一题目变体(无解情况)时上层判断。
	return []int{}
}
