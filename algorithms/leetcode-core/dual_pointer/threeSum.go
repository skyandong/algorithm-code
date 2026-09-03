package dualpointer

import "sort"

// 三数之和
// https://leetcode.cn/problems/3sum/description/
//
// 题意: 找出数组中所有和为 0 且不重复的三元组 [a, b, c]。
//
// 为什么用「排序 + 双指针」而不是哈希表?
//   - 哈希表做法: 固定一个数, 用两数之和(哈希)找剩下两个。复杂度也是 O(n^2),
//     但需要 O(n) 额外空间, 而且去重很麻烦(三元组要排序后去重, 否则 [a,b,c]
//     的排列会重复计入)。
//   - 排序 + 双指针: 排序天然让相同元素相邻, 去重只需要看「相邻是否相等」,
//     一行就能搞定; 且双指针是原地扫描, O(1) 额外空间。
//   - 所以双指针不仅复杂度最优, 去重也最干净 —— 这是本题的标准最优解。
//
// 复杂度:
//   - 时间: O(n^2)。外层枚举 first 走 n, 内层双指针扫 O(n)。排序 O(n log n) 被覆盖。
//   - 空间: O(1) 额外空间(不算排序递归的 O(log n) 栈空间)。
//   - 三数之和的理论下界就是 O(n^2): 必须枚举第一个数, 剩下两数至少 O(n) 查找,
//     再低不可能。所以这已经是渐进最优。
func threeSum(nums []int) [][]int {
	// 第一步: 排序。排序是整个解法的基础 —— 它让双指针的「左小右大」成立,
	// 也让去重变成「看相邻元素是否相等」这一简单动作。
	sort.Ints(nums)
	n := len(nums)
	var result [][]int

	// 边界: 不足三个数, 不可能凑出三元组。
	if n < 3 {
		return result
	}

	// 枚举第一个数 nums[first]。first 从 0 开始, 最多到 n-3
	// (后面至少要留两个数给 second/third)。
	for first := 0; first < n; first++ {
		// ===== 剪枝 1: 当前第一个数已 > 0, 后面更大, 三数之和不可能 = 0 =====
		// 为什么是 > 0 而不是 >= 0? 因为 first 本身可以是 0:
		// 形如 [0, 0, 0] 的解是合法的, 数组里有 0 时仍可能凑出 0。
		// 只有当前数严格 > 0 时, 排序后的三数之和才必然 > 0, 此时 break 安全。
		// 写成 >= 0 会漏掉全 0 解。
		if nums[first] > 0 {
			break
		}

		// ===== 去重 1: 跳过重复的第一个数 =====
		// 关键点: 比较的是 nums[first] == nums[first-1] (看前一个), 不是 first+1。
		//
		// 为什么必须看「前一个」? 反例: 输入 [-1, -1, 2]。
		//   - first=0 时 nums[0]=-1, 这是第一个 -1, 必须尝试配对(它能和 2、
		//     另一个 -1 凑成 0)。
		//   - 如果去重写成 nums[first] == nums[first+1], first=0 时发现
		//     nums[1] 也是 -1, 就 continue 跳过了 —— 于是 [-1,-1,2] 这个合法解被漏掉。
		//   - 正确做法: first>0 且等于「上一个已经处理过的」nums[first-1] 时才跳过,
		//     因为「上一个相同值」的解已经在上轮 first 全部找完了, 这次再找只会重复。
		//
		// 一句话: 去重要跳过的是「已经处理过的重复」, 不是「还没处理的下一个」。
		if first > 0 && nums[first] == nums[first-1] {
			continue
		}

		// 双指针: second 指向 first 右边第一个, third 指向数组末尾。
		// 因为数组已排序, nums[second] 向右递增, nums[third] 向左递减。
		second, third := first+1, n-1
		for second < third {
			sum := nums[first] + nums[second] + nums[third]
			if sum == 0 {
				// 命中一组解。
				result = append(result, []int{nums[first], nums[second], nums[third]})

				// ===== 命中后必须同时移动 second 和 third, 不能只移一个 =====
				// 为什么? 此刻 sum=0, 固定 first 时, second 和 third 是唯一满足的一对。
				//   - 只移 second(++)、third 不动: nums[third] 没变, 而 second 变了,
				//     新的 sum 不可能再 = 0(因为之前的 second 是唯一能让 sum=0 的值),
				//     循环会进入 sum<0 分支继续右移 second —— 但这趟是白跑的,
				//     且如果数组里有重复值, 下一轮还可能把同一组解重复 append。
				//   - 同时移动才能既推进指针、又避免重复记录同一组合。
				second++
				third--

				// ===== 命中后去重 2: 跳过 second 侧的重复值 =====
				// 命中并 second++ 后, 如果新的 nums[second] 还等于上一轮的值,
				// 凑出来的还是同一组解, 必须继续右移跳过。
				// 比较的是 nums[second] == nums[second-1] —— 同样是看「刚处理过的前一个」。
				for second < third && nums[second] == nums[second-1] {
					second++
				}
				// 同理, third 侧向左跳过重复值, 比较 nums[third] == nums[third+1]。
				for second < third && nums[third] == nums[third+1] {
					third--
				}
			} else if sum < 0 {
				// 和太小, 需要变大 → second 右移(nums[second] 变大)。
				second++
			} else {
				// 和太大, 需要变小 → third 左移(nums[third] 变小)。
				third--
			}
		}
	}
	return result
}
