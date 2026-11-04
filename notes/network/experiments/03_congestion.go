// # 流控实验（笔记 03）
//
// 对应笔记：notes/network/03-可靠传输与拥塞控制.md
//
// 运行：go run ./experiments/ congestion
package main

import (
	"fmt"
	"io"
	"net"
	"time"
)

// ExpCongestion 演示「对端不读 → 发送方 Write 阻塞」，这是接收窗口（rwnd）耗尽，
// 与丢包导致的拥塞窗口收缩是两回事。
func ExpCongestion() {
	slow, err := writeWithSlowReader(400 * time.Millisecond)
	if err != nil {
		fmt.Println("实验失败：", err)
		return
	}
	fast, err := writeWithSlowReader(0)
	if err != nil {
		fmt.Println("实验失败：", err)
		return
	}

	fmt.Printf("① 服务端收到连接后先不读，停 400ms：1MB 写完耗时 %v\n", slow.Round(time.Millisecond))
	fmt.Printf("② 服务端收到连接后立刻读：          1MB 写完耗时 %v\n", fast.Round(time.Millisecond))
	fmt.Println()
	fmt.Println("判读：")
	fmt.Println("  Write 调用被卡住，是因为接收方窗口（rwnd）用完了——对端应用没有 read，")
	fmt.Println("  内核接收缓冲填满后通告窗口 0，发送方只能停下等 WINDOW_UPDATE。")
	fmt.Println("  这与「链路丢包 → cwnd 砍半」是两种完全不同的现象：")
	fmt.Println("    rwnd 问题是接收方应用慢（本实验）")
	fmt.Println("    cwnd 问题是网络有损（需要 ss -ti 看 retrans 才能确认）")
}

// writeWithSlowReader 起一个本地服务端：延迟 delay 后再读，返回客户端写满 1MB 的耗时。
func writeWithSlowReader(delay time.Duration) (time.Duration, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if delay > 0 {
			time.Sleep(delay)
		}
		io.Copy(io.Discard, conn)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return 0, fmt.Errorf("不是 TCP 连接")
	}
	// 发送缓冲压小，让 rwnd 耗尽更快地体现为 Write 阻塞
	if err := tcpConn.SetWriteBuffer(4096); err != nil {
		return 0, err
	}

	payload := make([]byte, 1<<20)
	start := time.Now()
	written := 0
	for written < len(payload) {
		n, err := conn.Write(payload[written:])
		written += n
		if err != nil {
			return time.Since(start), err
		}
	}
	return time.Since(start), nil
}
