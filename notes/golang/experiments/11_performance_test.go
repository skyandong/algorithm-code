// 实验 11（性能调优实战），断言验证
// 对应笔记：notes/golang/10-性能调优实战.md
// 源码对照：src/testing/benchmark.go（b.Loop）
// 计时对比不进断言（不稳定），统一放 Benchmark
package main

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ===== 实验实现（原 11_performance.go，已并入本文件）=====

// 包级 sink：防止「结果未被使用」的分配/计算被编译器优化掉（benchmark 同款手法）。
var (
	perfSinkBytes  []byte
	perfSinkString string
	perfSinkSlice  []int
	perfSinkInt    int
)

// perfBig 4KB 大结构体，用于值/指针传参对比。
type perfBig struct{ data [4096]byte }

//go:noinline
func perfSumByValue(x perfBig) int { return int(x.data[0]) + int(x.data[1]) }

//go:noinline
func perfSumByPtr(x *perfBig) int { return int(x.data[0]) + int(x.data[1]) }

// perfAllocDelta 返回 f 的耗时与执行期间的累计堆分配字节（TotalAlloc 差值）。
// 先 runtime.GC() 拿到干净基线；TotalAlloc 只增不减，差值即本段新增分配。
func perfAllocDelta(f func()) (time.Duration, uint64) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	t0 := time.Now()
	f()
	d := time.Since(t0)
	runtime.ReadMemStats(&after)
	return d, after.TotalAlloc - before.TotalAlloc
}

// ===== 断言用例 =====

func TestPreallocBeatsDynamicAppend(t *testing.T) {
	const n = 500_000

	_, dynAlloc := perfAllocDelta(func() {
		s := make([]int, 0) // 无 cap：反复扩容，旧数组变垃圾
		for i := 0; i < n; i++ {
			s = append(s, i)
		}
		perfSinkSlice = s
	})
	_, preAlloc := perfAllocDelta(func() {
		s := make([]int, 0, n) // 预分配：零扩容
		for i := 0; i < n; i++ {
			s = append(s, i)
		}
		perfSinkSlice = s
	})

	assert.GreaterOrEqual(t, dynAlloc, uint64(n*8), "动态 append：累计分配不少于最终大小")
	assert.LessOrEqual(t, preAlloc, uint64(n*8*3/2), "预分配：只有一次分配")
	assert.Less(t, preAlloc, dynAlloc, "预分配累计分配明显更少（约 1/2）")
}

func TestPoolBeatsPerCallAlloc(t *testing.T) {
	if raceEnabled {
		t.Skip("-race 下 race runtime 自身的分配会污染 TotalAlloc 读数，池化收益无法量化")
	}

	const (
		iters   = 2000
		bufSize = 64 << 10 // 64KB
	)
	pool := &sync.Pool{New: func() any { return make([]byte, bufSize) }}

	_, newAlloc := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			buf := make([]byte, bufSize)
			buf[0] = byte(i)
			perfSinkBytes = buf
		}
	})
	_, poolAlloc := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			buf := pool.Get().([]byte)
			buf[0] = byte(i)
			pool.Put(buf)
		}
	})

	assert.GreaterOrEqual(t, newAlloc, uint64(iters*bufSize/2), "每次 make：累计分配 64MB 以上")
	assert.Less(t, poolAlloc, newAlloc/100, "sync.Pool 复用：分配量降两个数量级")
}

func TestStringByteConversionAlloc(t *testing.T) {
	const iters = 20000
	s := strings.Repeat("hello, 世界", 800) // ~10.4KB
	b := []byte(s)
	m := map[string]int{s: 1}

	_, toBytes := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			perfSinkBytes = []byte(s) // string→[]byte：每次分配 + 拷贝
		}
	})
	_, toStr := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			perfSinkString = string(b) // []byte→string：每次分配 + 拷贝
		}
	})
	_, mapIdx := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			perfSinkInt = m[string(b)] // 编译器认得的零拷贝场景
		}
	})

	half := uint64(len(s)) * iters / 2
	assert.GreaterOrEqual(t, toBytes, half, "[]byte(s) 每次分配 + 拷贝")
	assert.GreaterOrEqual(t, toStr, half, "string(b) 每次分配 + 拷贝")
	assert.Less(t, mapIdx, uint64(1024), "m[string(b)] 是编译器特例：索引免分配")
}

func TestGCNextGCIsTwiceLive(t *testing.T) {
	perfSinkBytes = make([]byte, 32<<20) // 撑大 live heap，比例观察更准

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	ratio := float64(ms.NextGC) / float64(ms.HeapAlloc)
	assert.Greater(t, ratio, 1.5, "GOGC=100：NextGC 约为 live 堆的 2 倍")
	assert.Less(t, ratio, 3.0)
}

func TestAllocationPressureTriggersGC(t *testing.T) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	before := ms.NumGC

	for i := 0; i < 128; i++ {
		g := make([]byte, 1<<20) // 1MB 瞬时垃圾，不留引用
		g[0] = byte(i)
	}
	runtime.ReadMemStats(&ms)

	assert.Greater(t, ms.NumGC, before, "分配压力自动驱动 GC，无需手动")
}

func BenchmarkPreallocVsAppend(b *testing.B) {
	const n = 10000

	b.Run("dynamic", func(b *testing.B) {
		for b.Loop() {
			s := make([]int, 0)
			for i := 0; i < n; i++ {
				s = append(s, i)
			}
			perfSinkSlice = s
		}
	})
	b.Run("prealloc", func(b *testing.B) {
		for b.Loop() {
			s := make([]int, 0, n)
			for i := 0; i < n; i++ {
				s = append(s, i)
			}
			perfSinkSlice = s
		}
	})
}

func BenchmarkValueVsPtr(b *testing.B) {
	x := perfBig{}

	b.Run("value-4KB", func(b *testing.B) {
		for b.Loop() {
			perfSinkInt = perfSumByValue(x)
		}
	})
	b.Run("pointer", func(b *testing.B) {
		for b.Loop() {
			perfSinkInt = perfSumByPtr(&x)
		}
	})
}
