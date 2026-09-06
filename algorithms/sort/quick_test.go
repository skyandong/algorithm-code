package sort

import (
	"math/rand"
	stdsort "sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuickSort(t *testing.T) {
	assert := assert.New(t)
	array := []int{53, 17, 78, 9, 45, 65, 87, 23, 31}
	QuickSort(array)
	assert.Equal([]int{9, 17, 23, 31, 45, 53, 65, 78, 87}, array)
}

// TestQuickSort_EdgeCases 空数组与单元素走 SortNonR(0, -1) 分支，最容易下标越界
func TestQuickSort_EdgeCases(t *testing.T) {
	assert := assert.New(t)

	empty := []int{}
	QuickSort(empty)
	assert.Equal([]int{}, empty)

	single := []int{42}
	QuickSort(single)
	assert.Equal([]int{42}, single)

	pair := []int{2, 1}
	QuickSort(pair)
	assert.Equal([]int{1, 2}, pair)

	pairSorted := []int{1, 2}
	QuickSort(pairSorted)
	assert.Equal([]int{1, 2}, pairSorted)
}

// TestQuickSort_Duplicates partSort3 只在 array[cur] < key 时交换，等于 key 的元素不动，
// 全重复数组是每个 partition 方案的经典边界
func TestQuickSort_Duplicates(t *testing.T) {
	assert := assert.New(t)

	allSame := []int{7, 7, 7, 7, 7, 7}
	QuickSort(allSame)
	assert.Equal([]int{7, 7, 7, 7, 7, 7}, allSame)

	manyDup := []int{3, 1, 3, 2, 1, 3, 2, 1}
	QuickSort(manyDup)
	assert.Equal([]int{1, 1, 1, 2, 2, 3, 3, 3}, manyDup)
}

// TestQuickSort_AlreadySorted 本实现固定取末位为 key，已排序数据会退化成 O(n²)。
// 这里保证的是「结果仍然正确」，性能退化由 BenchmarkQuickSort 量化。
func TestQuickSort_AlreadySorted(t *testing.T) {
	assert := assert.New(t)
	array := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	QuickSort(array)
	assert.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}, array)
}

func TestQuickSort_ReverseSorted(t *testing.T) {
	assert := assert.New(t)
	array := []int{9, 8, 7, 6, 5, 4, 3, 2, 1}
	QuickSort(array)
	assert.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}, array)
}

func TestQuickSort_Negatives(t *testing.T) {
	assert := assert.New(t)
	array := []int{-5, 3, 0, -12, 8, -1}
	QuickSort(array)
	assert.Equal([]int{-12, -5, -1, 0, 3, 8}, array)
}

// TestQuickSort_AgainstStdlib 随机 500 组与标准库对拍，比手写用例更能覆盖划分的边界
func TestQuickSort_AgainstStdlib(t *testing.T) {
	assert := assert.New(t)
	r := rand.New(rand.NewSource(42))

	for i := 0; i < 500; i++ {
		n := r.Intn(64) // 含 0，覆盖空数组
		got := make([]int, n)
		want := make([]int, n)
		for j := range got {
			v := r.Intn(100) - 50
			got[j], want[j] = v, v
		}
		QuickSort(got)
		stdsort.Ints(want)
		assert.Equal(want, got, "第 %d 组失败，输入长度 %d", i, n)
	}
}

// TestSortNonR 显式指定区间，验证只排 [begin, end] 而不动两侧
func TestSortNonR(t *testing.T) {
	assert := assert.New(t)
	array := []int{9, 5, 3, 7, 1, 8, 2}
	SortNonR(array, 1, 4) // 只排下标 1..4
	assert.Equal([]int{9, 1, 3, 5, 7, 8, 2}, array)
}
