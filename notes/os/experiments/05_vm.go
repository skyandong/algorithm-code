// # 虚拟内存与缺页实验（笔记 05）
//
// 对应笔记：notes/os/05-虚拟内存与缺页.md
//
// 运行：go run ./experiments/ vm
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// faults 是进程累计的缺页次数。
type faults struct {
	minor int64
	major int64
}

// ExpVM 观察「申请不等于占用」与 mmap 文件的缺页行为。
func ExpVM() {
	pageSize := os.Getpagesize()
	const anonSize = 64 << 20

	before := readFaults()

	data, err := syscall.Mmap(-1, 0, anonSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		fmt.Println("mmap 匿名内存失败：", err)
		return
	}
	afterAlloc := readFaults()
	fmt.Printf("① mmap %d MB 匿名内存后（一次都没写）：minor +%d，major +%d\n",
		anonSize>>20, afterAlloc.minor-before.minor, afterAlloc.major-before.major)
	fmt.Println("   判读：只登记了虚拟地址，物理页一张都没分配——所以快")

	for i := 0; i < anonSize; i += pageSize {
		data[i] = 1
	}
	afterTouch := readFaults()
	fmt.Printf("   逐页写一遍（%d 页）：minor +%d，major +%d\n",
		anonSize/pageSize, afterTouch.minor-afterAlloc.minor, afterTouch.major-afterAlloc.major)
	fmt.Println("   判读：每页一次 minor fault——页在物理内存里，只是缺映射")

	if err := syscall.Munmap(data); err != nil {
		fmt.Println("munmap 失败：", err)
	}

	// 文件映射：读一遍看 major 还是 minor
	dir, err := os.MkdirTemp("", "os-vm")
	if err != nil {
		fmt.Println("创建临时目录失败：", err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "mapped.bin")
	if err := writeFile(path, 8<<20); err != nil {
		fmt.Println("写文件失败：", err)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		fmt.Println("打开文件失败：", err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		fmt.Println("取文件大小失败：", err)
		return
	}

	mapped, err := syscall.Mmap(int(f.Fd()), 0, int(info.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		fmt.Println("mmap 文件失败：", err)
		return
	}
	defer syscall.Munmap(mapped)

	beforeRead := readFaults()
	sum := 0
	for i := 0; i < len(mapped); i += pageSize {
		sum += int(mapped[i])
	}
	afterRead := readFaults()
	fmt.Printf("\n② mmap 一个刚写完的 8MB 文件后逐页读：minor +%d，major +%d（sum=%d）\n",
		afterRead.minor-beforeRead.minor, afterRead.major-beforeRead.major, sum)
	fmt.Println("   判读：文件刚写入，内容还在页缓存里，所以只产生 minor fault；")
	fmt.Println("   若文件是冷的（没被读过、缓存已被回收），这里会出现 major fault——那才是真读盘")

	fmt.Println()
	fmt.Println("结论：VmSize/VSS 会显示你 mmap 了 64MB，但 RSS 只有真正触碰过的页；")
	fmt.Println("      两者之间的差就是「申请了但没碰」的部分。")
}

// readFaults 读取进程累计的缺页次数。
func readFaults() faults {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return faults{minor: ru.Minflt, major: ru.Majflt}
}

// writeFile 写一个指定大小的文件。
func writeFile(path string, size int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	chunk := make([]byte, 1<<20)
	for i := range chunk {
		chunk[i] = 'A' // 写入非零内容，读映射时能看出确实读到了数据
	}
	for written := 0; written < size; written += len(chunk) {
		if _, err := f.Write(chunk); err != nil {
			return err
		}
	}
	return f.Close()
}
