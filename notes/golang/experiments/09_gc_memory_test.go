// 实验 09（内存管理与 GC），断言验证
// 对应笔记：notes/golang/08-内存管理与GC.md
// 源码对照：src/runtime/mheap.go
package main

import (
	"runtime"
	"runtime/debug"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ===== 实验实现（原 09_gc_memory.go，已并入本文件）=====

// gcSink 强制逃逸：写入包级变量的对象必须堆分配，防止编译器优化掉 new。
var gcSink *[64]byte

// gcAnySink 用于演示 interface 参数装箱逃逸。
var gcAnySink any

// 下面六个函数供 -gcflags="-m -l" 观察逃逸判定（-l 禁用内联使判定稳定）。

// escapeReturnPtr 返回局部变量指针：栈帧没了还要用，只能上堆
//
//go:noinline
func escapeReturnPtr() *int {
	x := 42
	return &x // 栈帧销毁后 x 还要被调用方使用 → 堆
}

// escapeClosure 返回闭包：被捕获的变量生命周期被延长
//
//go:noinline
func escapeClosure() func() int {
	x := 42
	return func() int { return x + 1 } // 闭包延长了 x 的生命周期 → 堆
}

// escapeDynamicSlice 动态长度 make：编译期定不下大小
//
//go:noinline
func escapeDynamicSlice(n int) []int {
	s := make([]int, n) // 编译期不知道 n，栈上无法预留 → 堆
	return s
}

// escapeInterfaceArg 参数存进全局：装箱对象逃逸
//
//go:noinline
func escapeInterfaceArg(v any) int {
	gcAnySink = v // 参数存入全局 → 装箱对象逃逸
	if n, ok := v.(int); ok {
		return n
	}
	return -1
}

// noEscapeFixedSum 大小已知且不外泄：留在栈上
//
//go:noinline
func noEscapeFixedSum() int {
	s := make([]int, 4) // 大小已知且生命周期封闭在本函数 → 栈
	s[0], s[1], s[2], s[3] = 1, 2, 3, 4
	return s[0] + s[1] + s[2] + s[3]
}

// escapeSendToChannel 指针跨 goroutine 传递：上堆
//
//go:noinline
func escapeSendToChannel(ch chan *int) {
	x := 1024
	ch <- &x // 指针跨 goroutine 传递 → 堆
}

// ===== 断言用例 =====

// TestEscapeScenariosBehave 第 1 节：六个逃逸场景的行为（判定看 -gcflags）
func TestEscapeScenariosBehave(t *testing.T) {
	assert.Equal(t, 42, *escapeReturnPtr())
	assert.Equal(t, 43, escapeClosure()())
	assert.Len(t, escapeDynamicSlice(8), 8)
	assert.Equal(t, 1024, escapeInterfaceArg(1024))
	assert.Equal(t, -1, escapeInterfaceArg("x"))
	assert.Equal(t, 10, noEscapeFixedSum())

	ch := make(chan *int, 1)
	go escapeSendToChannel(ch)
	assert.Equal(t, 1024, *<-ch, "指针跨 goroutine 传递（逃逸场景之一）")
}

// TestNoEscapeFixedSumAllocatesNothing 用零堆分配证明「不逃逸」
func TestNoEscapeFixedSumAllocatesNothing(t *testing.T) {
	_ = noEscapeFixedSum() // 预热

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 1000; i++ {
		_ = noEscapeFixedSum()
	}
	runtime.ReadMemStats(&after)

	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(1024),
		"大小已知且不外泄 → 栈分配，零堆分配（这就是「不逃逸」的硬证据）")
}

// TestEscapeReturnPtrAllocates 用堆分配证明指针确实逃逸
func TestEscapeReturnPtrAllocates(t *testing.T) {
	_ = escapeReturnPtr() // 预热

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 1000; i++ {
		_ = escapeReturnPtr()
	}
	runtime.ReadMemStats(&after)

	assert.GreaterOrEqual(t, after.TotalAlloc-before.TotalAlloc, uint64(1000*8),
		"返回局部变量指针 → 每次堆分配一个 int")
}

// TestMemStatsAllocGrowsAndGCReclaims 第 2 节：TotalAlloc 只增，GC 后 Alloc 回落
func TestMemStatsAllocGrowsAndGCReclaims(t *testing.T) {
	var ms runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&ms)
	baseAlloc := ms.Alloc
	baseTotal := ms.TotalAlloc

	for i := 0; i < 512*1024; i++ { // 512K × 64B = 32MB
		b := new([64]byte)
		b[0] = byte(i)
		gcSink = b // 覆盖旧引用：前面的对象全部变成垃圾
	}

	runtime.ReadMemStats(&ms)
	assert.Greater(t, ms.TotalAlloc, baseTotal, "TotalAlloc 只增不减（累计分配量）")
	assert.GreaterOrEqual(t, ms.TotalAlloc-baseTotal, uint64(16<<20), "本轮累计分配 16MB 以上")

	runtime.GC()
	runtime.ReadMemStats(&ms)
	assert.Less(t, ms.Alloc, baseAlloc+uint64(4<<20), "手动 GC 后 Alloc 回落（垃圾被三色标记清扫）")
}

// TestSyncPoolBeatsNewAlloc 第 3 节：池化把分配量降两个数量级
func TestSyncPoolBeatsNewAlloc(t *testing.T) {
	if raceEnabled {
		t.Skip("-race 下 race runtime 自身的分配会污染 TotalAlloc 读数，池化收益无法量化")
	}

	const rounds = 1 << 20

	runtime.GC()
	var ms1, ms2 runtime.MemStats
	runtime.ReadMemStats(&ms1)
	for i := 0; i < rounds; i++ {
		b := new([64]byte)
		b[0] = byte(i)
		gcSink = b
	}
	runtime.ReadMemStats(&ms2)
	newAlloc := ms2.TotalAlloc - ms1.TotalAlloc

	pool := sync.Pool{New: func() any { return new([64]byte) }}
	runtime.GC() // 清空池，从头计数
	var ms3, ms4 runtime.MemStats
	runtime.ReadMemStats(&ms3)
	for i := 0; i < rounds; i++ {
		b := pool.Get().(*[64]byte)
		b[0] = byte(i)
		pool.Put(b)
	}
	runtime.ReadMemStats(&ms4)
	poolAlloc := ms4.TotalAlloc - ms3.TotalAlloc

	assert.GreaterOrEqual(t, newAlloc, uint64(rounds*64/2), "每次 new：累计分配 32MB 以上")
	assert.Less(t, poolAlloc, newAlloc/100, "sync.Pool 复用：分配量降两个数量级")
}

// TestGOGCPercentToggle 第 4 节：GOGC=off 不触发 GC，恢复后手动 GC 生效
func TestGOGCPercentToggle(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过（要分配 128MB 大对象）")
	}

	prev := debug.SetGCPercent(-1) // -1 = GOGC=off，返回修改前的值
	assert.GreaterOrEqual(t, prev, -1)
	defer debug.SetGCPercent(prev)

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	for i := 0; i < 4; i++ { // 128MB 大对象（>32KB 直接走 mheap 页堆）
		b := make([]byte, 32<<20)
		b[0] = byte(i)
		_ = b
	}
	runtime.ReadMemStats(&after)

	assert.Equal(t, before.NumGC, after.NumGC, "GOGC=off 期间分配 128MB 也不触发 GC")

	runtime.GC() // 手动触发
	runtime.ReadMemStats(&after)
	assert.Greater(t, after.NumGC, before.NumGC, "手动 GC 生效，堆回落")

	assert.Greater(t, debug.SetMemoryLimit(-1), int64(0), "GOMEMLIMIT 只读查询（软上限）")
}
