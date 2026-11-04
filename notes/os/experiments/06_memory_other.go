//go:build !linux

// # 内核内存账本（笔记 06，非 Linux 平台）
//
// 对应笔记：notes/os/06-内存回收与OOM.md
package main

import "fmt"

// readKernelMem 在非 Linux 平台只给出取数方式：/proc 是 Linux 专有。
func readKernelMem() {
	fmt.Println("\n⑤ 内核侧账本：本机不是 Linux，没有 /proc/meminfo 与 cgroup v2")
	fmt.Println("   macOS 上的等价命令：")
	fmt.Println("     vm_stat                                  # 页统计：active/wired/compressed")
	fmt.Println("     memory_pressure                          # 内存压力与 available 估算")
	fmt.Println("     sysctl hw.memsize                        # 物理内存总量")
	fmt.Println("   Linux 上会打印 MemAvailable/Cached/AnonPages 与 cgroup memory.current/max，")
	fmt.Println("   并给出「file 可回收、anon 才是真占用」的判读依据。")
}
