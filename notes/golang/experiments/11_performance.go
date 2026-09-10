// # 性能调优实战实验
//
// 对应笔记：notes/golang/10-性能调优实战.md
//
// 本实验为断言式单元测试（无打印），断言见 11_performance_test.go：
//
//	go test -run 'TestPrealloc|TestSyncPool|TestStringByte|TestGC|TestAllocation' ./experiments/
//	go test -bench 'BenchmarkPreallocVsAppend|BenchmarkValueVsPtr' -benchmem ./experiments/
//
// 线上排查路径（症状 → 工具 → 动作）与 pprof/benchstat 命令见笔记 10 第 6/8 节。
//
// —— runtime 源码对照 ——
//
// src/testing/benchmark.go（b.Loop 的设计要点，简化示意）
//
//	func (b *B) Loop() bool {
//		// 首次调用：决定总轮数并开始计时；此后每次调用返回 true
//		// 全部结束：只统计循环体耗时——setup 天然排除在计时外，
//		//          且循环体结果被框架持有，编译器无法把它优化掉
//	}
//
// 经典路径对照：b.N 手动翻倍猜轮数 + ResetTimer/StopTimer 手工圈定计时区间
package main

import (
	"runtime"
	"time"
)

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
