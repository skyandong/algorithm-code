package arrays

import (
	"sort"
)

func mergeBySort(nums1 []int, m int, nums2 []int, n int) {
	copy(nums1[m:], nums2)
	sort.Slice(nums1, func(i, j int) bool {
		return nums1[i] < nums1[j]
	})
}

func mergeByDualPointer(nums1 []int, m int, nums2 []int, n int) {
	// 计算合并后数组的总长度
	totalLen := m + n

	// 双指针从合并后数组的尾部开始向前遍历
	for i := totalLen - 1; m > 0 && n > 0; i-- {
		num1 := nums1[m-1]
		num2 := nums2[n-1]

		// 将较大的数字放入合并后数组的末尾，并移动对应指针
		if num2 >= num1 {
			nums1[i] = num2
			n--
		} else {
			nums1[i] = num1
			m--
		}
	}

	// 如果 nums2 中还有剩余元素，将其复制到 nums1 的开头
	if n > 0 {
		copy(nums1[:n], nums2[:n])
	}
}
