//go:build linux

// # 内核内存账本（笔记 06，仅 Linux）
//
// 对应笔记：notes/os/06-内存回收与OOM.md
package main

import (
	"fmt"
	"os"
	"strings"
)

// readKernelMem 打印 /proc/meminfo 与 cgroup v2 的内存账本。
func readKernelMem() {
	fmt.Println("\n⑤ 内核侧账本（/proc/meminfo）")
	info, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		fmt.Println("   读取 /proc/meminfo 失败：", err)
		return
	}
	want := map[string]bool{
		"MemTotal": true, "MemFree": true, "MemAvailable": true,
		"Cached": true, "AnonPages": true, "Dirty": true,
		"SwapTotal": true, "SwapFree": true,
	}
	for _, line := range strings.Split(string(info), "\n") {
		for key := range want {
			if strings.HasPrefix(line, key+":") {
				fmt.Println("  ", strings.TrimSpace(line))
				delete(want, key)
			}
		}
		if len(want) == 0 {
			break
		}
	}

	fmt.Println("\n⑥ cgroup v2 账本（容器里的真实上限）")
	printCgroup("/sys/fs/cgroup/memory.current", "memory.current")
	printCgroup("/sys/fs/cgroup/memory.max", "memory.max")
	printCgroup("/sys/fs/cgroup/memory.high", "memory.high")
	printCgroup("/sys/fs/cgroup/memory.events", "memory.events")

	stat, err := os.ReadFile("/sys/fs/cgroup/memory.stat")
	if err != nil {
		fmt.Println("   memory.stat 读不到（可能不是 cgroup v2）：", err)
		return
	}
	for _, line := range strings.Split(string(stat), "\n") {
		for _, key := range []string{"anon ", "file ", "slab "} {
			if strings.HasPrefix(line, key) {
				fmt.Println("   memory.stat:", strings.TrimSpace(line))
			}
		}
	}
	fmt.Println("   判读：file 是可回收的页缓存，anon 才是真正压死内存的部分；")
	fmt.Println("        memory.events 里 max 持续增长说明一直在顶上限跑，离 OOM 不远。")
}

// printCgroup 打印一个 cgroup 文件的内容（不存在就跳过）。
func printCgroup(path, label string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("   %s: 不存在（非 cgroup v2 或未限制）\n", label)
		return
	}
	fmt.Printf("   %s: %s", label, strings.TrimSpace(strings.ReplaceAll(string(data), "\n", " | ")))
	fmt.Println()
}
