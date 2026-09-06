package sort

import (
	"fmt"
	"math/rand"
	stdsort "sort"
	"testing"
)

// 跑法：
//
//	# 快速冒烟（推荐，单测走真实数据）
//	go test -bench=. -benchmem -benchtime=100x -run=^$ ./algorithms/sort/
//
//	# 拿精确数字（StdSort sorted/reverse 单条参考 ~0.7s，但全量套件含
//	# 退化的 O(n²) QuickSort sorted/reverse，单条要 1~2s，且 StopTimer/
//	# StartTimer 模式在极快函数（<1µs/op）下会让 framework 的 b.N 校准偏长，
//	# 全量跑完大约 5~10 分钟）
//	go test -bench=. -benchmem -benchtime=1s -run=^$ ./algorithms/sort/
//
// 为什么分三种数据分布：
// 本实现的快排固定取末位元素为 key（partSort3），对已排序/逆序数据每次划分只能排除
// 一个元素，退化成 O(n²)。只看随机数据会得出"快排最快"的错误结论——这正是面试时
// 被追问"快排什么时候会退化"的实证来源。
const (
	kindRandom  = "random"
	kindSorted  = "sorted"
	kindReverse = "reverse"
)

// genData 生成指定分布的数据，固定种子保证多次运行结果可比
func genData(n int, kind string) []int {
	r := rand.New(rand.NewSource(42))
	data := make([]int, n)
	switch kind {
	case kindRandom:
		for i := range data {
			data[i] = r.Intn(n*10) + 1
		}
	case kindSorted:
		for i := range data {
			data[i] = i
		}
	case kindReverse:
		for i := range data {
			data[i] = n - i
		}
	default:
		panic("unknown kind: " + kind)
	}
	return data
}

type sortFunc func([]int)

// benchSort 每次迭代都从同一份预生成数据重建输入。
// 重建用 StopTimer 排除在计时外，保证 ns/op 只反映排序本身的开销。
func benchSort(b *testing.B, fn sortFunc, kind string, sizes []int) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("%s/n=%d", kind, n), func(b *testing.B) {
			src := genData(n, kind)
			data := make([]int, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				copy(data, src)
				b.StartTimer()
				fn(data)
			}
		})
	}
}

// O(n²) 的冒泡只跑到 1000，再大单次迭代就要上百毫秒
func BenchmarkBubbleSort(b *testing.B) {
	benchSort(b, BubbleSort, kindRandom, []int{100, 500, 1000})
	benchSort(b, BubbleSort, kindSorted, []int{100, 500, 1000})
	benchSort(b, BubbleSort, kindReverse, []int{100, 500, 1000})
}

func BenchmarkInsertSort(b *testing.B) {
	benchSort(b, InsertSort, kindRandom, []int{100, 1000, 5000})
	benchSort(b, InsertSort, kindSorted, []int{100, 1000, 5000})
	benchSort(b, InsertSort, kindReverse, []int{100, 1000, 5000})
}

func BenchmarkShallSort(b *testing.B) {
	benchSort(b, ShallSort, kindRandom, []int{1000, 10000, 100000})
	benchSort(b, ShallSort, kindSorted, []int{1000, 10000, 100000})
	benchSort(b, ShallSort, kindReverse, []int{1000, 10000, 100000})
}

func BenchmarkHeapSort(b *testing.B) {
	benchSort(b, HeapSort, kindRandom, []int{1000, 10000, 100000})
	benchSort(b, HeapSort, kindSorted, []int{1000, 10000, 100000})
	benchSort(b, HeapSort, kindReverse, []int{1000, 10000, 100000})
}

func BenchmarkMergerSort(b *testing.B) {
	benchSort(b, MergerSort, kindRandom, []int{1000, 10000, 100000})
	benchSort(b, MergerSort, kindSorted, []int{1000, 10000, 100000})
	benchSort(b, MergerSort, kindReverse, []int{1000, 10000, 100000})
}

// BenchmarkQuickSort 有序场景刻意只跑到 10000：
// n=100000 的退化规模单次迭代要几秒，benchmark 会跑不完。
// 退化趋势在 1000→5000→10000 上已经能看清（耗时按 n² 增长）。
func BenchmarkQuickSort(b *testing.B) {
	benchSort(b, QuickSort, kindRandom, []int{1000, 10000, 100000})
	benchSort(b, QuickSort, kindSorted, []int{1000, 5000, 10000})
	benchSort(b, QuickSort, kindReverse, []int{1000, 5000, 10000})
}

// BenchmarkStdSort 标准库 pdqsort 作为参照系。
// 手写实现和标准库的差距，比五种手写实现之间的差距更有面试价值。
func BenchmarkStdSort(b *testing.B) {
	benchSort(b, stdsort.Ints, kindRandom, []int{1000, 10000, 100000})
	benchSort(b, stdsort.Ints, kindSorted, []int{1000, 10000, 100000})
	benchSort(b, stdsort.Ints, kindReverse, []int{1000, 10000, 100000})
}
