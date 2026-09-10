// # 内存管理与 GC 实验
//
// 对应笔记：notes/golang/08-内存管理与GC.md
//
// 本实验为断言式单元测试（无打印），断言见 09_gc_memory_test.go：
//
//	go test -run 'TestEscape|TestNoEscape|TestMemStats|TestSyncPool|TestGOGC' ./experiments/
//	go build -gcflags="-m -l" ./experiments/ 2>&1 | grep -E "escapes|moved to heap"
//
// —— runtime 源码对照 ——
//
// src/runtime/mheap.go（堆内存的分配/回收单位，简化示意）
//
//	type mspan struct {
//		next       *mspan    // 链表
//		spanclass  spanClass // 大小类别 + 是否含指针（决定是否需要扫描）
//		nelems     uintptr   // 槽位数（按 sizeclass 定长切分）
//		allocBits  *gcBits   // 分配位图
//		gcmarkBits *gcBits   // 标记位图——三色标记最终落到这里
//		// sweep 阶段：allocBits = gcmarkBits，清零 markBits
//	}
package main

// gcSink 强制逃逸：写入包级变量的对象必须堆分配，防止编译器优化掉 new。
var gcSink *[64]byte

// gcAnySink 用于演示 interface 参数装箱逃逸。
var gcAnySink any

// 下面六个函数供 -gcflags="-m -l" 观察逃逸判定（-l 禁用内联使判定稳定）。
//
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
	gcAnySink = v // 参数存入全局 → 装箱对象逃逸
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
