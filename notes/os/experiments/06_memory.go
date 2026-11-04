// # 内存账本实验（笔记 06）
//
// 对应笔记：notes/os/06-内存回收与OOM.md
//
// 运行：go run ./experiments/ memory
package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// ExpMemory 观察 Go 的堆指标与「还给 OS」之间的关系。
func ExpMemory() {
	reportMem("① 起始")

	const n = 200_000
	keep := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		keep = append(keep, make([]byte, 1024))
	}
	runtime.KeepAlive(keep)
	reportMem("② 分配 20 万个 1KB 对象（约 200MB）后")

	runtime.GC()
	runtime.KeepAlive(keep)
	reportMem("③ 对象仍被引用，runtime.GC() 之后")

	keep = nil
	runtime.GC()
	debug.FreeOSMemory()
	reportMem("④ 放弃引用 + GC + FreeOSMemory() 之后")

	fmt.Println()
	fmt.Println("判读：")
	fmt.Println("  HeapAlloc 是活对象，HeapIdle 是「已释放但还握在 Go 手里」的页，")
	fmt.Println("  HeapReleased 是真正 madvise 归还给内核的部分；Sys 是向 OS 要过的总量。")
	fmt.Println("  常见误判：GC 之后 HeapAlloc 降了但进程 RSS 不降 —— 中间隔着 HeapIdle。")
	fmt.Println("  Go 1.16 起默认用 MADV_DONTNEED 归还，RSS 会比较及时地跟着降；")
	fmt.Println("  1.12~1.15 用的是 MADV_FREE，RSS 要等内核有内存压力才回落。")

	readKernelMem()
}

// reportMem 打印 Go runtime 的内存指标。
func reportMem(label string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Printf("%s\n  HeapAlloc=%s HeapInuse=%s HeapIdle=%s HeapReleased=%s Sys=%s\n",
		label,
		mb(ms.HeapAlloc), mb(ms.HeapInuse), mb(ms.HeapIdle), mb(ms.HeapReleased), mb(ms.Sys))
}

// mb 把字节数格式化成 MB。
func mb(b uint64) string {
	return fmt.Sprintf("%6.1fMB", float64(b)/(1<<20))
}
