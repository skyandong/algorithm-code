// # 性能调优实战实验
//
// 对应笔记：notes/golang/11-性能调优实战.md
//
// 运行（接入 main.go 后）：
//
//	go run ./experiments/ perf
//
// 实验项：
//
//	第1节：预分配 vs 动态 append（耗时 + TotalAlloc 双维度）
//	第2节：sync.Pool 复用 vs 每次新分配（耗时 + TotalAlloc）
//	第3节：string ↔ []byte 转换的分配量（含 m[string(b)] 零拷贝对照）
//	第4节：大结构体值传递 vs 指针传递（//go:noinline 排除内联干扰）
//	第5节：GC 触发观测（NextGC ≈ 2×live 的 GOGC 语义 + NumGC 增长）
//	第6节：标准 benchmark / pprof 命令行提示（main 包跑不了 go test）
package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
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

// RunPerformanceExperiments 演示笔记 11 的性能调优要点。
func RunPerformanceExperiments() {
	fmt.Println("========== 第1节: 预分配 vs 动态 append ==========")
	perfPreallocVsAppend()

	fmt.Println("\n========== 第2节: sync.Pool 复用 vs 每次新分配 ==========")
	perfSyncPool()

	fmt.Println("\n========== 第3节: string ↔ []byte 转换的分配成本 ==========")
	perfStringByteConv()

	fmt.Println("\n========== 第4节: 大结构体值传递 vs 指针传递 ==========")
	perfValueVsPtr()

	fmt.Println("\n========== 第5节: GC 触发观测（GOGC 语义） ==========")
	perfGCObservation()

	fmt.Println("\n========== 第6节: 标准 benchmark 与 pprof 命令提示 ==========")
	perfBenchmarkGuide()
}

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

// perfPreallocVsAppend 第1节：100 万元素：动态 append（反复扩容）vs 一次到位。
func perfPreallocVsAppend() {
	const n = 1_000_000

	dA, allocA := perfAllocDelta(func() {
		s := make([]int, 0) // 无 cap：底层扩容 ~20 次，旧数组变垃圾
		for i := 0; i < n; i++ {
			s = append(s, i)
		}
		perfSinkSlice = s // 赋给包级 sink：既防优化，也强制堆分配保证计入 TotalAlloc
	})
	dB, allocB := perfAllocDelta(func() {
		s := make([]int, 0, n) // 预分配 cap：零扩容
		for i := 0; i < n; i++ {
			s = append(s, i)
		}
		perfSinkSlice = s
	})

	fmt.Printf("动态 append   : 耗时 %10v  累计分配 %8.1f MB\n", dA, float64(allocA)/1e6)
	fmt.Printf("预分配 cap    : 耗时 %10v  累计分配 %8.1f MB\n", dB, float64(allocB)/1e6)
	fmt.Println("结论：耗时差数倍是常态；动态扩容累计分配 ≈ 2×最终大小（1+2+4+…+N），预分配只有 1 次")
}

// perfSyncPool 第2节：10 万次 64KB buffer：每次 make vs sync.Pool 借还。
func perfSyncPool() {
	const (
		iters   = 100_000
		bufSize = 64 << 10 // 64KB
	)
	pool := &sync.Pool{
		New: func() any { return make([]byte, bufSize) },
	}

	dA, allocA := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			buf := make([]byte, bufSize) // 每次新分配
			buf[0] = byte(i)
			perfSinkBytes = buf // 包级 sink：防优化 + 强制堆分配
		}
	})
	dB, allocB := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			buf := pool.Get().([]byte) // 借
			buf[0] = byte(i)
			pool.Put(buf) // 还；GC 两轮后池内对象仍会被清空，pool 不是缓存
		}
	})

	fmt.Printf("每次 make    : 耗时 %10v  累计分配 %8.1f MB\n", dA, float64(allocA)/1e6)
	fmt.Printf("sync.Pool    : 耗时 %10v  累计分配 %8.1f MB\n", dB, float64(allocB)/1e6)
	fmt.Printf("结论：%.1f GB 的累计分配几乎全部消失（仅剩 New 触发的少量 miss 分配）；\n",
		float64(iters*bufSize)/1e9)
	fmt.Println("      注意：对象要够大（≥几 KB）且高频复用才值得池化，小对象池开销可能反超")
}

// perfStringByteConv 第3节：string ↔ []byte 往返转换的分配量 + 零拷贝场景对照。
func perfStringByteConv() {
	const iters = 100_000
	s := strings.Repeat("hello, 世界", 800) // ~10.4KB
	b := []byte(s)
	m := map[string]int{s: 1}

	d1, a1 := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			perfSinkBytes = []byte(s) // string→[]byte：每次分配 + 拷贝
		}
	})
	d2, a2 := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			perfSinkString = string(b) // []byte→string：每次分配 + 拷贝
		}
	})
	d3, a3 := perfAllocDelta(func() {
		for i := 0; i < iters; i++ {
			perfSinkInt = m[string(b)] // 编译器认得的零拷贝场景：不分配
		}
	})

	fmt.Printf("[]byte(s) ×100k     : 耗时 %10v  累计分配 %8.1f MB\n", d1, float64(a1)/1e6)
	fmt.Printf("string(b) ×100k     : 耗时 %10v  累计分配 %8.1f MB\n", d2, float64(a2)/1e6)
	fmt.Printf("m[string(b)] ×100k  : 耗时 %10v  累计分配 %8.1f MB\n", d3, float64(a3)/1e6)
	fmt.Println("结论：前两者每次转换都分配+拷贝；map 索引 / == 比较 / switch 是编译器认得的零拷贝场景")
	fmt.Println("红线：unsafe.String 做零拷贝后严禁修改原 []byte（string 不可变性是语言级契约）")
}

// perfValueVsPtr 第4节：4KB 结构体跨函数传参：值传递（栈复制）vs 指针传递。
func perfValueVsPtr() {
	const iters = 500_000
	x := perfBig{}

	// 加 //go:noinline 模拟真实跨函数调用：小函数默认会被内联，内联后复制可能被整体省掉
	dA, _ := perfAllocDelta(func() {
		sum := 0
		for i := 0; i < iters; i++ {
			sum += perfSumByValue(x) // 每次调用复制 4KB
		}
		perfSinkInt += sum
	})
	dB, _ := perfAllocDelta(func() {
		sum := 0
		for i := 0; i < iters; i++ {
			sum += perfSumByPtr(&x) // 只传 8B 指针
		}
		perfSinkInt += sum
	})

	fmt.Printf("值传递 4KB ×500k : 耗时 %10v\n", dA)
	fmt.Printf("指针传递   ×500k : 耗时 %10v\n", dB)
	fmt.Println("结论：大结构体传值每次复制 4KB，指针便宜得多；")
	fmt.Println("      但 &x 若被调用方长期持有会逃逸到堆（一次分配 + GC 追踪），取舍要实测：<64B 值传递无感")
}

// perfGCObservation 第5节：验证 GOGC=100 语义（NextGC ≈ 2×live）与分配压力驱动 GC。
func perfGCObservation() {
	limit := debug.SetMemoryLimit(-1) // 负数：只读取当前 soft limit，不修改
	fmt.Printf("当前 GOMEMLIMIT soft limit: %.0f MB\n", float64(limit)/1e6)

	// 制造 32MB 存活数据，让 live heap 足够大、比例观察更准
	perfSinkBytes = make([]byte, 32<<20)

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	live := ms.HeapAlloc
	fmt.Printf("强制 GC 后: HeapAlloc(live)=%.1f MB, NextGC=%.1f MB, NumGC=%d\n",
		float64(live)/1e6, float64(ms.NextGC)/1e6, ms.NumGC)
	fmt.Printf("→ NextGC/live ≈ %.1f×（GOGC=100：live 堆增长到约 2 倍即触发下一轮 GC）\n",
		float64(ms.NextGC)/float64(live))

	// 分配 256MB 瞬时垃圾（不留引用）：观察 GC 自动触发频率，不手动 GC
	before := ms.NumGC
	for i := 0; i < 256; i++ {
		g := make([]byte, 1<<20) // 1MB 垃圾
		g[0] = byte(i)            // 写一下保证分配不被优化掉
	}
	runtime.ReadMemStats(&ms)
	fmt.Printf("分配 256MB 瞬时垃圾后: NumGC %d → %d（分配压力自动驱动 GC，无需手动）\n",
		before, ms.NumGC)
	fmt.Println("结论：分配速率越高 GC 越频繁——控制分配速率就是控制 GC 频率（GOGC 只调节触发线）")
}

// perfBenchmarkGuide 第6节：标准 benchmark / pprof 命令提示（main 包跑不了 go test）。
func perfBenchmarkGuide() {
	fmt.Println(`标准 benchmark 写法（独立 _test.go 文件，main 包无法运行 go test）：

    var sink any // 包级 sink：防止死代码被优化（Go 1.24+ 用 b.Loop 可省）

    func BenchmarkWork(b *testing.B) {
        setup()
        for b.Loop() { // Go 1.24+：自动管理 N、排除 setup 计时、防循环体被优化
            sink = work()
        }
    }

常用命令：
    go test -bench=. -benchmem ./...
    go test -bench=BenchmarkWork -benchtime=2s -count=10 ./...
    benchstat old.txt new.txt    # 统计显著性对比（go install golang.org/x/perf/cmd/benchstat@latest）
    go build -gcflags='-m' ./... # 逃逸分析

线上 pprof（服务需 import _ "net/http/pprof" 并监听 debug 端口）：
    go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30        # CPU
    go tool pprof -sample_index=alloc_space http://localhost:6060/debug/pprof/heap  # 分配压力
    curl 'http://localhost:6060/debug/pprof/goroutine?debug=2' > g.txt        # goroutine 泄漏`)
}
