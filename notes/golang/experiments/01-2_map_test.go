// 实验 01（map 部分），断言验证；slice 部分见 01-1_slice_test.go
// 对应笔记：notes/golang/01-slice、string与map底层.md 第 11~14 节
// 源码对照：src/internal/runtime/maps/{map,table,group}.go
package main

import (
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// buildMap 造 n 个元素的 map，hint 控制是否预分配
func buildMap(n, hint int) map[int]int {
	m := make(map[int]int, hint)
	for i := 0; i < n; i++ {
		m[i] = i
	}
	return m
}

// TestMapPrealloc 第 11 节：预分配 hint 省掉 rehash 搬迁
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

// TestMapDeleteMemory 第 11 节：删除不缩容，删空才重置表
func TestMapDeleteMemory(t *testing.T) {
	const n = 100000

	runtime.GC()
	var msFull, msPartial, msEmpty, msDrop runtime.MemStats

	m := buildMap(n, n)
	runtime.GC()
	runtime.ReadMemStats(&msFull)

	for i := 0; i < 2*n/3; i++ {
		delete(m, i)
	}
	runtime.GC()
	runtime.ReadMemStats(&msPartial)

	for k := range m {
		delete(m, k)
	}
	runtime.GC()
	runtime.ReadMemStats(&msEmpty)

	m = nil
	runtime.GC()
	runtime.ReadMemStats(&msDrop)

	assert.Greater(t, int64(msPartial.HeapAlloc), int64(msFull.HeapAlloc)*9/10,
		"部分删除不缩容：占用仍在删除前的 90% 以上（tombstone 占位，槽位复用）")
	assert.Greater(t, int64(msPartial.HeapAlloc), int64(msEmpty.HeapAlloc),
		"部分删除的占用远高于删空（实测约 10 倍）")
	assert.Less(t, int64(msEmpty.HeapAlloc), int64(msFull.HeapAlloc), "删空后重置释放表内存")
	assert.LessOrEqual(t, int64(msDrop.HeapAlloc), int64(msEmpty.HeapAlloc)*2,
		"m=nil 后无可测量增长（与删空同量级；表已在删空时重置，此处仅释放 map 头）")
}

// TestMapUnordered 第 12 节：遍历顺序随机
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

// TestRangeMutation 第 12 节：range 期间删除与新增的语义
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
					delete(m, j)
				}
			}
		}
	}
	assert.Equal(t, 1, visited, "range 中删光未遍历的 key：被删的不再产出")

	m2 := map[int]int{}
	for i := 0; i < 10; i++ {
		m2[i] = i
	}
	count := 0
	for k := range m2 {
		if k == 5 {
			m2[100] = 100 // 新增：产出与否规范不保证，只断言不 panic
		}
		count++
	}
	assert.GreaterOrEqual(t, count, 10)
}

// TestConcurrentMapWriteFatal 第 13 节：并发写 map 是 fatal（子进程验证）
func TestConcurrentMapWriteFatal(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过子进程崩溃验证")
	}
	if raceEnabled {
		t.Skip("-race 下 race detector 会先报竞态并终止子进程，拿不到 runtime 的 fatal error 输出")
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestConcurrentMapWriteFatalChild", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLICEMAP_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()

	assert.Error(t, err, "fatal error 令子进程非零退出")
	assert.Contains(t, string(out), "fatal error: concurrent map writes")
	assert.NotContains(t, string(out), "panic:", "throw 不是 panic")
}

// TestConcurrentMapWriteFatalChild 子进程实体：制造并发写触发 fatal
func TestConcurrentMapWriteFatalChild(t *testing.T) {
	if os.Getenv("SLICEMAP_FATAL_CHILD") != "1" {
		t.Skip("仅由 TestConcurrentMapWriteFatal 以子进程拉起")
	}

	runtime.GOMAXPROCS(2) // 单核 runner 上也要真并发，否则写操作可能完全不重叠、不触发检测

	m := map[int]int{}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = recover() }() // recover 拦不住 throw
			<-start                          // 发令枪：4 个 goroutine 同时开始写
			for i := 0; i < 100000; i++ {
				m[i] = 1
			}
		}()
	}
	close(start)
	wg.Wait()
}

// TestSyncMapSemantics 第 14 节：sync.Map 的读写与 Range
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

// BenchmarkSyncMapRead 对比 RWMutex 版：读多写少场景 sync.Map 胜出
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

// BenchmarkRWMutexMapRead 读路径对照基准：map + RWMutex
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
