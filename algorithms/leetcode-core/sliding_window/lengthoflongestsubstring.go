package slidingwindow

// LengthOfLongestSubstring 无重复字符的最长子串
// https://leetcode-cn.com/problems/longest-substring-without-repeating-characters/
//
// 题意: 给定字符串 s，找出其中不含重复字符的最长子串的长度。
//       注意是"子串"(连续)，不是"子序列"(可不连续)，这一区别经常被面试官当场考。
//
// 解法选择理由:
//   - 暴力: 枚举所有子串 O(n^2) 再判重 O(n) => O(n^3)，n=5e4 必超时，不可行。
//   - 二分+check: 可做但绕远，且 check 仍是 O(n)，整体 O(n log n)，不是最优。
//   - 滑动窗口(本解法): 左右指针 begin/end 维护一个"无重复字符"的窗口，
//     end 右扩，遇到重复就把 begin 跳到重复字符上次出现位置的下一位，
//     借助 map 记录字符最近一次出现的下标，整体 O(n)。
//   - 还可配合"当窗口内字符种类已覆盖全部可能字符时提前收拢"等剪枝，
//     但对纯 ASCII 字符集意义不大，反而增加常数，故不引入。
//
// 复杂度:
//   - 时间 O(n): end 单调右移 n 次，begin 也只单调右移(从不回退)，累计最多 n 次。
//     两个指针都不回头是 O(n) 的关键，面试官常追问"为什么不是 O(n^2)"——
//     答案就是 begin 只增不减，均摊到每个元素至多被 begin/end 各访问一次。
//   - 空间 O(min(n, |Σ|)): map 最多存字符集大小。这里用 uint8 字节为 key，
//     |Σ|≤256，是常数级；若按 rune 处理 Unicode 则 |Σ| 可能更大但仍受 n 上界约束。
//   - 是否理论下界: 时间已是下界——至少要把每个字符看一遍 Ω(n)，无法更优；
//     空间用 map 已是渐近最优，常数上比定长数组 [128]int 略大但更通用。
func LengthOfLongestSubstring(s string) int {
	// 用 map 记录"字符 -> 最近一次出现的下标"。
	// key 选 uint8 而非 rune: 直接按字节索引 s[end]，省一次类型转换，对纯 ASCII 输入正确。
	// 【坑】若输入含中文等多字节 UTF-8 字符，按字节去重会破坏"按字符去重"的语义:
	//       一个中文字符占 3 字节，会被当作 3 个不同 key，可能漏判重复。
	//       LeetCode 这道题的判题数据用此写法可通过，但面试中若被问到 Unicode 场景,
	//       应说明需要改成 rune 切片 `[]rune(s)` 再做。
	mymap := make(map[uint8]int)

	// begin/end 是窗口的左右闭边界; maxLen 记录历史最大窗口长度。
	// 初始化全为 0: 空串时直接走到末尾返回 0, 无需特判。
	var maxLen, begin, end int

	// end 每轮右移一格, 扩展窗口右端。
	for ; end < len(s); end++ {
		// 查 map: 当前字符 s[end] 之前是否出现过(index, ok)。
		// 【最关键的坑】必须额外判断 `index >= begin`:
		//   map 里可能存着"该字符在 begin 之前的旧位置"(stale index)。
		//   若不加这个判断, 在形如 "abba" 的输入上会出错——
		//     处理到第二个 b 时 begin=2, map[b]=1, 若直接 begin=1+1=2 还算对;
		//     但处理到第二个 a 时 begin 已是 2, map[a]=0, 0 < begin,
		//     若不判断就会把 begin 退回到 0+1=1, 窗口反而变大且包含重复字符, 答案错误。
		//   所以只有当重复字符的位置仍在当前窗口内(index >= begin)时, 才需要收缩 begin。
		//   "去重方向"一定是向右收缩左边界, 绝不能向左回退。
		if index, ok := mymap[s[end]]; ok && index >= begin {
			// 在收缩前先用当前窗口长度 end-begin 更新答案。
			// 这里窗口长度是 end-begin(不含 end 本身), 因为 s[end] 即将与窗口内字符重复,
			// 真正无重复的窗口是 [begin, end), 长度恰为 end-begin。
			if max := end - begin; max > maxLen {
				maxLen = max
			}
			// 把左边界跳到"重复字符上次出现位置的下一位", 一次性跳过冲突段,
			// 而不是逐格右移 begin——这是把 O(n^2) 优化到 O(n) 的核心。
			begin = index + 1
		}
		// 无论是否命中重复, 都要更新当前字符的最新下标。
		// 【顺序坑】必须放在 begin 调整之后, 否则存入的是会被立刻跳过的旧值逻辑错乱;
		//         且 map 始终保留"该字符最近一次出现位置", 供后续 end 再次遇到时查询。
		mymap[s[end]] = end
	}

	// 【容易漏的一步】循环内只在"遇到重复时"更新答案, 但最长子串可能延伸到字符串末尾,
	//   末尾没有重复字符触发更新, 此时循环结束后窗口 [begin, end) 仍是一个有效候选。
	//   例如 s="abc", 循环内从未进入 if 分支, maxLen 一直是 0, 必须靠这里补算 end-begin=3。
	//   end 此时等于 len(s), 所以 end-begin 正是末尾窗口长度。
	if end-begin > maxLen {
		maxLen = end - begin
	}
	return maxLen
}
