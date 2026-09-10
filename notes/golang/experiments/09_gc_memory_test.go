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
