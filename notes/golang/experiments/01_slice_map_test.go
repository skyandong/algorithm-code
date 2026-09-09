// 实验 01（slice 与 map）—— 全部结论用断言验证，无打印观察。
//
// 对应笔记：notes/golang/01-slice与map底层.md
// 跑法：go test ./experiments/ -run 'Slice|Map|Append|Delete|Growth|Subslice|Range|Sync' -v
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

// dataPtr 取 slice 底层数组首地址（断言指针关系的通用工具）。
func dataPtr[T any](s []T) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(s)))
}

// TestSliceHeader 第 1 节：slice 是 (ptr, len, cap) 三字段头；
// 子切片/赋值共享底层数组，拷贝的是头不是数据。
func TestSliceHeader(t *testing.T) {
	s := []int{10, 20, 30} // 字面量：len=cap=3
	assert.Len(t, s, 3)
	assert.Equal(t, 3, cap(s))

	sub := s[1:]
	assert.Len(t, sub, 2)
	assert.Equal(t, 2, cap(sub))
	assert.Equal(t, dataPtr(s)+unsafe.Sizeof(s[0]), dataPtr(sub),
		"s[1:] 的数据指针 = 原 array + 1×元素宽度：新 header 指向同一数组中间")

	sub[0] = 99
	assert.Equal(t, 99, s[1], "两个 header 共享一个数组")

	s2 := s
	assert.Equal(t, dataPtr(s), dataPtr(s2), "赋值只拷贝 (ptr,len,cap)")
}

// TestAppendSharedAndSeparated 第 2 节：未超 cap 的 append 写共享数组；
// 超 cap 扩容换新数组后才分离。
func TestAppendSharedAndSeparated(t *testing.T) {
	a := make([]int, 3, 6)

	b := append(a, 99)
	assert.Equal(t, dataPtr(a), dataPtr(b), "未超 cap：b 与 a 共享底层数组")

	c := append(a, 100)
	assert.Equal(t, 100, b[3], "两次 append 写同一槽位，c 顶掉 b 的 99")

	d := append(c, 1, 2, 3, 4)
	assert.NotEqual(t, dataPtr(c), dataPtr(d), "len=8 > cap=6：扩容分配新数组")
	assert.GreaterOrEqual(t, cap(d), 8)
}

// TestAppendInside 第 2.3 节：函数内 append 只改副本头的 len。
func TestAppendInside(t *testing.T) {
	s := make([]int, 0, 4)
	appendInside(s)

	assert.Len(t, s, 0, "调用方 len 不变")
	assert.Equal(t, []int{7}, s[:1], "数据其实写进了共享数组")
}

// TestAppendAndReturn 第 2.3 节：正确姿势是返回新 header。
func TestAppendAndReturn(t *testing.T) {
	got := appendAndReturn(make([]int, 0, 4))

	assert.Equal(t, []int{7}, got)
}

// growthSink 包级变量：让 append 逃逸，走 runtime.growslice 真实路径
// （非逃逸小 append 会被编译器分配 ≤32 字节的栈上背衬数组，观察不到规律）。
var growthSink []int

// TestGrowthCapSequence 第 3 节：cap<256 精确翻倍；
// 之后增长系数落在 [1.25, 2]（平滑公式 + sizeclass 对齐）。
func TestGrowthCapSequence(t *testing.T) {
	growthSink = make([]int, 0)
	growthSink = append(growthSink, 1) // 首跳 cap 0→1：newLen > 2*oldCap 的按需分配分支
	prev := cap(growthSink)
	for i := 2; i <= 10000; i++ {
		growthSink = append(growthSink, i)
		if cap(growthSink) == prev {
			continue
		}
		if prev < 256 {
			assert.Equal(t, 2*prev, cap(growthSink), "翻倍段")
		} else {
			ratio := float64(cap(growthSink)) / float64(prev)
			assert.GreaterOrEqual(t, ratio, 1.25, "平滑公式下限")
			assert.LessOrEqual(t, ratio, 2.0, "sizeclass 对齐只往上修")
		}
		prev = cap(growthSink)
	}
	assert.GreaterOrEqual(t, cap(growthSink), 10000)
}

// TestDeleteWays 第 4 节：三种删除写法的残留差异与 a[len(a)] 越界反面教材。
func TestDeleteWays(t *testing.T) {
	// 截断：O(1)，但尾部值仍占着底层数组（指针元素会泄漏）
	a := []int{1, 2, 3, 4}
	a = a[:len(a)-1]
	assert.Equal(t, []int{1, 2, 3}, a)
	assert.Equal(t, 4, a[:cap(a)][len(a)], "截断不清槽位")

	// copy 覆盖：保序，尾部残留旧值
	b := []int{1, 2, 3, 4, 5}
	b = append(b[:1], b[2:]...)
	assert.Equal(t, []int{1, 3, 4, 5}, b)
	assert.Equal(t, 5, b[:cap(b)][len(b)])

	// swap-delete：O(1) 不保序，被删位置立即覆盖、无尾部残留
	c := []int{1, 2, 3, 4, 5}
	c[1] = c[len(c)-1]
	c = c[:len(c)-1]
	assert.Equal(t, []int{1, 5, 3, 4}, c)

	// 指针元素：截断前先清槽位，被删对象才可 GC
	v := 42
	p := []*int{&v, &v, &v}
	p[len(p)-1] = nil
	p = p[:len(p)-1]
	assert.Nil(t, p[:cap(p)][len(p)])

	// 反面教材：截断后写 a[len(a)] → index out of range（索引上限是 len-1）
	func() {
		defer func() {
			assert.Contains(t, fmt.Sprint(recover()), "index out of range")
		}()
		q := make([]*int, 2, 2)
		q = q[:1]
		q[len(q)] = nil
	}()
}

// TestSubsliceLeak 第 4.4 节：小切片挂住整个大数组，copy 出独立小片才释放。
func TestSubsliceLeak(t *testing.T) {
	big := make([]byte, 1<<20) // 1 MiB
	tail := big[len(big)-2:]

	tp, bp := dataPtr(tail), dataPtr(big)
	assert.Greater(t, tp, bp)
	assert.Less(t, tp, bp+1<<20, "tail 指向 big 内部：tail 活着，1 MiB 就回收不了")

	fixed := make([]byte, len(tail))
	copy(fixed, tail)
	fp := dataPtr(fixed)
	assert.True(t, fp < bp || fp >= bp+1<<20, "copy 修复后 fixed 独立于 big")
}

// buildMap 按 size hint 构建 n 元素 map。
func buildMap(n, hint int) map[int]int {
	m := make(map[int]int, hint)
	for i := 0; i < n; i++ {
		m[i] = i
	}
	return m
}

// TestMapPrealloc 第 5 节前半：预分配一次到位，累计分配字节显著少于动态增长。
// 用 TotalAlloc 而不是计时：断言要稳定可重复。
func TestMapPrealloc(t *testing.T) {
	const n = 100000

	runtime.GC()
	var ms0, ms1, ms2 runtime.MemStats
	runtime.ReadMemStats(&ms0)
	noHint := buildMap(n, 0)
	runtime.ReadMemStats(&ms1)
	withHint := buildMap(n, n)
	runtime.ReadMemStats(&ms2)

	assert.Equal(t, n, len(noHint))
	assert.Equal(t, n, len(withHint))
	assert.Less(t, int(ms2.TotalAlloc-ms1.TotalAlloc), int(ms1.TotalAlloc-ms0.TotalAlloc),
		"预分配的累计分配字节应更少（少 rehash 搬迁）")
}

// TestMapDeleteMemory 第 5 节后半：部分删除不缩容；删空 Swiss 重置释放；m=nil 彻底回收。
func TestMapDeleteMemory(t *testing.T) {
	const n = 100000

	runtime.GC()
	var msFull, msPartial, msEmpty, msDrop runtime.MemStats

	m := buildMap(n, n)
	runtime.GC()
	runtime.ReadMemStats(&msFull)
	heapFull := int64(msFull.HeapAlloc)

	for i := 0; i < 2*n/3; i++ {
		delete(m, i)
	}
	runtime.GC()
	runtime.ReadMemStats(&msPartial)
	assert.GreaterOrEqual(t, int64(msPartial.HeapAlloc), heapFull,
		"部分删除不缩容（tombstone 占位，槽位复用）")

	for k := range m {
		delete(m, k)
	}
	runtime.GC()
	runtime.ReadMemStats(&msEmpty)
	assert.Less(t, int64(msEmpty.HeapAlloc), heapFull, "删空后重置释放表内存")

	m = nil
	runtime.GC()
	runtime.ReadMemStats(&msDrop)
	assert.LessOrEqual(t, int64(msDrop.HeapAlloc), int64(msEmpty.HeapAlloc), "m=nil 彻底回收")
}

// TestMapUnordered 第 6 节：range 起点随机——顺序会变，内容始终是同一批 key。
func TestMapUnordered(t *testing.T) {
	m := map[int]int{}
	for i := 0; i < 10; i++ {
		m[i] = i
	}
	collect := func() []int {
		out := make([]int, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}

	first := collect()
	changed := false
	for i := 0; i < 20; i++ {
		next := collect()
		assert.ElementsMatch(t, first, next, "同一批 key，只是顺序不同")
		if !slices.Equal(first, next) {
			changed = true
		}
	}
	assert.True(t, changed, "21 次遍历顺序全相同的概率可忽略")
}

// TestRangeMutation 第 7 节：range 中删除安全（未遍历的不再产出）；新增不保证可见。
func TestRangeMutation(t *testing.T) {
	m := map[int]int{}
	for i := 0; i < 10; i++ {
		m[i] = i
	}
	visited := 0
	for k := range m {
		visited++
		if visited == 1 {
			for j := 0; j < 10; j++ {
				if j != k {
					delete(m, j) // 删光所有「尚未遍历」的 key
				}
			}
		}
	}
	assert.Equal(t, 1, visited, "规范保证：被删的 key 不会再产出")

	// 新增：可能产出也可能跳过，规范不保证——只断言不 panic、原 key 全遍历
	m2 := map[int]int{}
	for i := 0; i < 10; i++ {
		m2[i] = i
	}
	count := 0
	for k := range m2 {
		if k == 5 {
			m2[100] = 100
		}
		count++
	}
	assert.GreaterOrEqual(t, count, 10)
}

// TestSyncMapSemantics 第 9 节：sync.Map 并发读写安全（真实 map 这样做会 fatal）。
// 读多写少 vs RWMutex 的性能对比不进断言（计时不可靠），放 Benchmark。
func TestSyncMapSemantics(t *testing.T) {
	var sm sync.Map

	_, ok := sm.Load("k")
	assert.False(t, ok)
	sm.Store("k", 1)
	v, ok := sm.Load("k")
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				sm.Store(i%16, i)
				sm.Load(i % 16)
			}
		}()
	}
	wg.Wait()
}

// BenchmarkSyncMapRead 第 9 节读多写少场景：go test -bench=SyncMapRead 对比。
func BenchmarkSyncMapRead(b *testing.B) {
	var sm sync.Map
	for i := 0; i < 16; i++ {
		sm.Store(i, i)
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sm.Load(1)
		}
	})
}

// BenchmarkRWMutexMapRead 与上面对照：全局 RWMutex 读路径。
func BenchmarkRWMutexMapRead(b *testing.B) {
	m := map[int]int{1: 1}
	var mu sync.RWMutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			_ = m[1]
			mu.RUnlock()
		}
	})
}

// TestConcurrentMapWriteFatal 第 8 节：并发写 map 是 throw 不是 panic——
// 本进程会直接崩，只能在子进程里验证（标准 subprocess 模式）。
func TestConcurrentMapWriteFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过子进程崩溃验证")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestConcurrentMapWriteFatalChild", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLICEMAP_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()

	assert.Error(t, err, "fatal error 令子进程非零退出")
	assert.Contains(t, string(out), "fatal error: concurrent map writes")
	assert.NotContains(t, string(out), "panic:", "throw 不是 panic")
}

// TestConcurrentMapWriteFatalChild 只作为上面用例的子进程运行。
func TestConcurrentMapWriteFatalChild(t *testing.T) {
	if os.Getenv("SLICEMAP_FATAL_CHILD") != "1" {
		t.Skip("仅由 TestConcurrentMapWriteFatal 以子进程拉起")
	}

	m := map[int]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }() // 演示 recover 拦不住 throw
			for i := 0; i < 1000; i++ {
				m[i] = 1
			}
		}()
	}
	wg.Wait()
}

// TestNilEmptySlice 第 10 节：nil 切片与空切片判空等价；头、==nil、JSON 三处不同。
func TestNilEmptySlice(t *testing.T) {
	var nilSlice []int    // header {nil, 0, 0}
	emptySlice := []int{} // header {zerobase, 0, 0}

	assert.Equal(t, 0, len(nilSlice))
	assert.Equal(t, 0, len(emptySlice), "判空一律 len()==0")

	assert.True(t, nilSlice == nil)
	assert.False(t, emptySlice == nil)

	assert.Nil(t, unsafe.SliceData(nilSlice))
	assert.NotNil(t, unsafe.SliceData(emptySlice), "空切片数据指针指向 zerobase")

	assert.True(t, reflect.ValueOf(nilSlice).IsNil())
	assert.False(t, reflect.ValueOf(emptySlice).IsNil())

	nb, _ := json.Marshal(nilSlice)
	eb, _ := json.Marshal(emptySlice)
	assert.Equal(t, "null", string(nb), "nil 切片 JSON 是 null")
	assert.Equal(t, "[]", string(eb), "空切片 JSON 是 []")
}
