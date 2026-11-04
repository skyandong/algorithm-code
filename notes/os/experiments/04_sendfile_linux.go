//go:build linux

// # sendfile 直发实验（笔记 04，仅 Linux）
//
// 对应笔记：notes/os/04-零拷贝与页缓存.md
package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// runSendfileDemo 用 syscall.Sendfile 把文件直接送进 socket：数据不经过用户态。
func runSendfileDemo(path string) {
	fmt.Printf("\n③ sendfile 直发（Linux 专有）：文件 → socket，数据不进用户态\n")

	f, err := os.Open(path)
	if err != nil {
		fmt.Println("   打开文件失败：", err)
		return
	}
	defer f.Close()

	addr, stop := startSinkServer()
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("   连接失败：", err)
		return
	}
	defer conn.Close()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		fmt.Println("   不是 TCP 连接，跳过")
		return
	}
	outFile, err := tcpConn.File() // 复制出阻塞模式的 fd 供 sendfile 使用
	if err != nil {
		fmt.Println("   取 fd 失败：", err)
		return
	}
	defer outFile.Close()

	start := time.Now()
	remaining := int64(zcFileSize)
	var offset int64
	for remaining > 0 {
		n, err := syscall.Sendfile(int(outFile.Fd()), int(f.Fd()), &offset, int(remaining))
		remaining -= int64(n)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			fmt.Println("   sendfile 失败：", err)
			return
		}
		if n == 0 {
			break
		}
	}
	fmt.Printf("   sendfile 发送 %d MB 耗时 %v（曲线：磁盘→页缓存 DMA，页缓存→网卡 DMA）\n",
		zcFileSize>>20, time.Since(start).Round(time.Millisecond))
	fmt.Println("   对比②：这条路径上没有任何一次 CPU 拷贝，也没有「用户缓冲」这个中间站")
}
