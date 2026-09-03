// # 内存管理与 GC 实验
//
// 对应笔记：notes/golang/09-内存管理与GC.md
//
// 运行（接入 main.go 后）：
//
//	go run ./experiments/ gcmemory
//
// 实验项：
//
//	第1节：逃逸分析典型场景 + -gcflags="-m" 验证命令提示
//	第2节：runtime.MemStats 观察分配与 GC（Alloc/TotalAlloc/HeapInuse/NumGC）
//	第3节：sync.Pool 复用 vs 每次新分配（TotalAlloc 与 NumGC 对比）
//	第4节：GOGC 语义（关闭 GC 分配不触发；恢复后手动 GC）+ GOMEMLIMIT 查询
package main

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// RunGCMemoryExperiments 演示笔记 9 的内存管理与 GC 行为。
func RunGCMemoryExperiments() {
	fmt.Println("========== 第1节: 逃逸分析典型场景 ==========")
	gcEscape()

	fmt.Println("\n========== 第2节: runtime.MemStats 观察分配与 GC ==========")
	gcMemStats()

	fmt.Println("\n========== 第3节: sync.Pool 复用 vs 每次新分配 ==========")
	gcSyncPool()

	fmt.Println("\n========== 第4节: GOGC 语义与 GOMEMLIMIT ==========")
	gcGOGC()
}

// gcEscape 第1节：五种典型场景，配合命令行 -gcflags="-m" 查看判定。
func gcEscape() {
	// 各函数用 //go:noinline 标注，保证逃逸判定不受内联影响
	p := escapeReturnPtr()        // 返回局部变量指针 → 逃逸
	f := escapeClosure()          // 闭包捕获 → 逃逸
	s := escapeDynamicSlice(8)    // 动态长度 make → 逃逸
	n := escapeInterfaceArg(1024) // interface 装箱 → 逃逸（0~255 有静态缓存例外）
	sum := noEscapeFixedSum()     // 固定大小且不外泄 → 栈
	ch := make(chan *int, 1)
	go escapeSendToChannel(ch) // 指针跨 goroutine → 逃逸
	sent := <-ch

	fmt.Printf("返回局部变量指针: *p = %d\n", *p)
	fmt.Printf("闭包捕获: f() = %d（被捕获的 x 逃逸）\n", f())
	fmt.Printf("动态长度 slice: len(s) = %d（make([]int, n) 逃逸）\n", len(s))
	fmt.Printf("interface 装箱: n = %d\n", n)
	fmt.Printf("发送指针到 channel: *sent = %d（x 逃逸）\n", *sent)
	fmt.Printf("固定大小不外泄: sum = %d（栈分配，零堆分配）\n", sum)
	fmt.Println(`验证命令: go build -gcflags="-m -l" ./experiments/ 2>&1 | grep -E "escapes|moved to heap"`)
	fmt.Println("（-l 禁用内联使判定稳定；逃逸不是坏事，只优化 profiling 证明热的路径）")
}

//go:noinline
func escapeReturnPtr() *int {
	x := 42
	return &x // 栈帧销毁后 x 还要被调用方使用 → 堆
}

//go:noinline
func escapeClosure() func() int {
	x := 42
	return func() int { return x + 1 } // 闭包延长了 x 的生命周期 → 堆
}

//go:noinline
func escapeDynamicSlice(n int) []int {
	s := make([]int, n) // 编译期不知道 n，栈上无法预留 → 堆
	return s
}

//go:noinline
func escapeInterfaceArg(v any) int {
	gcAnySink = v // 参数存入全局 → 装箱对象逃逸（只做类型断言不外泄则不逃逸）
	if n, ok := v.(int); ok {
		return n
	}
	return -1
}

//go:noinline
func noEscapeFixedSum() int {
	s := make([]int, 4) // 大小已知且生命周期封闭在本函数 → 栈
	s[0], s[1], s[2], s[3] = 1, 2, 3, 4
	return s[0] + s[1] + s[2] + s[3]
}

//go:noinline
func escapeSendToChannel(ch chan *int) {
	x := 1024
	ch <- &x // 指针跨 goroutine 传递 → 堆
}

// gcSink 强制逃逸：写入包级变量的对象必须堆分配，防止编译器优化掉 new。
var gcSink *[64]byte

// gcAnySink 用于演示 interface 参数装箱逃逸。
var gcAnySink any

// gcMemStats 第2节：分配 32MB 垃圾，观察累计分配、堆与 GC 轮数。
func gcMemStats() {
	var ms runtime.MemStats

	runtime.ReadMemStats(&ms)
	printMemStats("基线       ", &ms)

	// 分配 ~32MB 的 64B 小对象并全部丢弃引用（产生垃圾）
	// 不写全局变量的话，new 会被优化成栈分配甚至完全消除，演示不出分配与 GC
	sink := 0
	for i := 0; i < 512*1024; i++ {
		b := new([64]byte)
		b[0] = byte(i)
		gcSink = b // 覆盖旧引用 → 前面的对象全部变成垃圾
		sink += int(b[0])
	}
	runtime.ReadMemStats(&ms)
	printMemStats("丢 32MB 后 ", &ms)
	fmt.Printf("（TotalAlloc 只增不减：累计分配量；NumGC 增长 = 垃圾到目标堆触发；sink=%d 防死代码消除）\n", sink)

	runtime.GC() // 手动触发一轮完整 GC
	runtime.ReadMemStats(&ms)
	printMemStats("手动 GC 后 ", &ms)
	fmt.Println("（Alloc 回落：无引用的 64B 对象被三色标记-清扫回收）")
}

func printMemStats(label string, ms *runtime.MemStats) {
	fmt.Printf("%s Alloc=%6.1f MB TotalAlloc=%7.1f MB HeapInuse=%6.1f MB NumGC=%d\n",
		label, memMB(ms.Alloc), memMB(ms.TotalAlloc), memMB(ms.HeapInuse), ms.NumGC)
}

// gcSyncPool 第3节：100 万次 64B 操作，对比每次 new 与 Get/Put 复用。
func gcSyncPool() {
	const rounds = 1 << 20 // 1048576 次 × 64B = 64MB 分配量（若不复用）
	var ms1, ms2, ms3, ms4 runtime.MemStats
	sink := byte(0)

	// 模式 A：每次 new，复用率为 0（写全局 gcSink 强制真实堆分配）
	runtime.GC()
	runtime.ReadMemStats(&ms1)
	t0 := time.Now()
	for i := 0; i < rounds; i++ {
		b := new([64]byte)
		b[0] = byte(i)
		gcSink = b
		sink ^= b[0]
	}
	newCost := time.Since(t0)
	runtime.ReadMemStats(&ms2)

	// 模式 B：sync.Pool Get/Put 循环复用
	pool := sync.Pool{New: func() any { return new([64]byte) }}
	runtime.GC() // 清空池，从头计数
	runtime.ReadMemStats(&ms3)
	t0 = time.Now()
	for i := 0; i < rounds; i++ {
		b := pool.Get().(*[64]byte)
		b[0] = byte(i)
		sink ^= b[0]
		pool.Put(b)
	}
	poolCost := time.Since(t0)
	runtime.ReadMemStats(&ms4)

	fmt.Printf("每次 new:       耗时 %9v  TotalAlloc +%7.1f MB  NumGC +%d\n",
		newCost, memMB(ms2.TotalAlloc-ms1.TotalAlloc), ms2.NumGC-ms1.NumGC)
	fmt.Printf("sync.Pool 复用: 耗时 %9v  TotalAlloc +%7.1f MB  NumGC +%d\n",
		poolCost, memMB(ms4.TotalAlloc-ms3.TotalAlloc), ms4.NumGC-ms3.NumGC)
	fmt.Println("（池命中时每个 P 只分配极少数对象：分配量骤降、GC 几乎不触发）")
	fmt.Println("（每轮 GC 会清空池，Go 1.13 起 victim 二级缓存把清空摊到两轮；高 churn 场景收益依然显著）")
	_ = sink
}

// gcGOGC 第4节：GOGC=off 期间分配不触发 GC；GOMEMLIMIT 只读查询。
func gcGOGC() {
	fmt.Println("GOGC 默认 100：目标堆 = 上轮存活堆 × (1 + GOGC/100)，另有 4MB 最小目标")
	prev := debug.SetGCPercent(-1) // -1 = GOGC=off，返回修改前的值
	fmt.Printf("debug.SetGCPercent(-1) 已关闭 GC 触发（原值 = %d）\n", prev)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	for i := 0; i < 4; i++ { // 分配 128MB 大对象（>32KB 直接走 mheap 页堆）
		b := make([]byte, 32<<20)
		b[0] = byte(i)
		_ = b
	}
	runtime.ReadMemStats(&after)
	fmt.Printf("关闭期间分配 128MB: NumGC %d -> %d，HeapInuse %.1f -> %.1f MB（堆疯涨但 GC 不触发）\n",
		before.NumGC, after.NumGC, memMB(before.HeapInuse), memMB(after.HeapInuse))

	debug.SetGCPercent(prev) // 恢复原值
	runtime.GC()
	runtime.ReadMemStats(&after)
	fmt.Printf("恢复后手动 runtime.GC(): NumGC = %d，HeapInuse = %.1f MB（垃圾回收，堆回落）\n",
		after.NumGC, memMB(after.HeapInuse))

	limit := debug.SetMemoryLimit(-1) // -1 只查询不修改
	fmt.Printf("当前 GOMEMLIMIT = %s（Go 1.19+，软上限：接近时 GC 提频用 CPU 换内存）\n", humanLimit(limit))
	fmt.Println("（容器常见组合：GOGC=off + GOMEMLIMIT=limit×80%，平时不主动 GC、逼近上限才收）")
}

func memMB(b uint64) float64 { return float64(b) / (1 << 20) }

func humanLimit(b int64) string {
	if b == math.MaxInt64 {
		return "未设置（math.MaxInt64）"
	}
	return fmt.Sprintf("%d 字节（约 %.2f GB）", b, float64(b)/(1<<30))
}
