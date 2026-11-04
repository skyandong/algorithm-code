// # 零拷贝与页缓存实验（笔记 04）
//
// 对应笔记：notes/os/04-零拷贝与页缓存.md
//
// 运行：go run ./experiments/ zerocopy
package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

const zcFileSize = 32 << 20

// ExpZeroCopy 观察两件事：页缓存命中带来的巨大差异，以及文件→socket 的拷贝路径。
func ExpZeroCopy() {
	dir, err := os.MkdirTemp("", "os-zerocopy")
	if err != nil {
		fmt.Println("创建临时目录失败：", err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "big.bin")
	if err := writeTestFile(path, zcFileSize); err != nil {
		fmt.Println("写测试文件失败：", err)
		return
	}

	first := timeReadFile(path)
	second := timeReadFile(path)
	fmt.Printf("① 顺序读 %d MB：第一次 %v，第二次 %v\n", zcFileSize>>20,
		first.Round(time.Millisecond), second.Round(time.Millisecond))
	throughput := func(d time.Duration) float64 {
		if d <= 0 {
			return 0
		}
		return float64(zcFileSize) / d.Seconds() / (1 << 30)
	}
	fmt.Printf("   吞吐：第一次 %.1f GB/s，第二次 %.1f GB/s\n", throughput(first), throughput(second))
	fmt.Println("   注意：这个文件是本实验刚写完的，所以两次都命中页缓存——第一次读并不「冷」。")
	fmt.Println("   这个量级的吞吐只有内存能解释，磁盘做不到：这正是 page cache 的作用。")
	fmt.Println("   想看真正的冷读，需要 root 执行 `echo 3 > /proc/sys/vm/drop_caches`（Linux）后再读，")
	fmt.Println("   或用 O_DIRECT 绕过页缓存；没有 root 时本实验能证明的是「热读=内存拷贝」。")

	viaCopy := timeCopyToSocket(path, false)
	viaBufio := timeCopyToSocket(path, true)
	fmt.Printf("② 文件 → socket：io.Copy %v，bufio 手动拷 %v\n",
		viaCopy.Round(time.Millisecond), viaBufio.Round(time.Millisecond))
	fmt.Println("   io.Copy 在「*os.File → *net.TCPConn」时可能走 sendfile；")
	fmt.Println("   套一层 bufio 就必然退回用户态拷贝——这正是零拷贝被无意破坏的典型写法")

	runSendfileDemo(path)
}

// writeTestFile 写一个指定大小的临时文件。
func writeTestFile(path string, size int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	chunk := make([]byte, 1<<20)
	for written := 0; written < size; written += len(chunk) {
		if _, err := f.Write(chunk); err != nil {
			return err
		}
	}
	return f.Sync()
}

// timeReadFile 完整读一遍文件，返回耗时。
func timeReadFile(path string) time.Duration {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	start := time.Now()
	io.Copy(io.Discard, f)
	return time.Since(start)
}

// timeCopyToSocket 把文件发到本地 sink 服务端，返回耗时。
// useBufio=true 时用 bufio 手动拷贝，破坏 sendfile 路径。
func timeCopyToSocket(path string, useBufio bool) time.Duration {
	addr, stop := startSinkServer()
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("连接 sink 服务端失败：", err)
		return 0
	}
	defer conn.Close()

	f, err := os.Open(path)
	if err != nil {
		fmt.Println("打开文件失败：", err)
		return 0
	}
	defer f.Close()

	start := time.Now()
	if useBufio {
		buf := make([]byte, 32<<10)
		r := bufio.NewReaderSize(f, 32<<10)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				if _, werr := conn.Write(buf[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	} else {
		io.Copy(conn, f)
	}
	return time.Since(start)
}

// startSinkServer 起一个只读不回的本地服务端，返回地址与关闭函数。
func startSinkServer() (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", func() {}
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				io.Copy(io.Discard, conn)
			}()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}
