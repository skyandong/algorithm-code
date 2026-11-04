// # 建连实验（笔记 01）
//
// 对应笔记：notes/network/01-TCP报文与三次握手.md
//
// 运行：go run ./experiments/ handshake
package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// ExpHandshake 演示一次成功建连，以及两种完全不同的失败：RST 拒绝 与 SYN 被静默丢弃。
func ExpHandshake() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("监听失败：", err)
		return
	}
	addr := ln.Addr().String()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("① 本机回环建连失败：%v\n", err)
	} else {
		fmt.Printf("① 建连成功：%s → %s，耗时 %v\n", conn.LocalAddr(), conn.RemoteAddr(), elapsed)
		fmt.Println("   应用拿到 conn 时三次握手已在内核完成：SYN / SYN+ACK / ACK")
		conn.Close()
	}
	ln.Close()

	fmt.Println("\n② 失败一：对端端口没监听 —— 内核立刻回 RST")
	start = time.Now()
	_, err = net.DialTimeout("tcp", "127.0.0.1:1", 500*time.Millisecond)
	fmt.Printf("   %-22v 耗时 %v\n", err, time.Since(start).Round(time.Millisecond))
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		fmt.Println("   判读：ECONNREFUSED = 收到 RST。进程没起或端口不对，重试无意义")
	case err == nil:
		fmt.Println("   判读：本机 1 端口居然有服务在听，换个端口再试")
	default:
		fmt.Println("   判读：不是 RST，按其它错误归因")
	}

	fmt.Println("\n③ 失败二：SYN 被中间设备丢弃 —— 只能等到超时")
	start = time.Now()
	_, err = net.DialTimeout("tcp", "10.255.255.1:9", 800*time.Millisecond)
	cost := time.Since(start).Round(time.Millisecond)
	switch {
	case err == nil:
		fmt.Printf("   本次 dial 竟然成功（耗时 %v）——说明本机有代理/网关替该地址应答了，\n", cost)
		fmt.Println("   这个网络环境下演示不出超时；在直连环境下会一直等到 deadline 并返回 i/o timeout")
	case os.IsTimeout(err):
		fmt.Printf("   i/o timeout 耗时 %v\n", cost)
		fmt.Println("   判读：timeout = 发出 SYN 后没有任何应答（防火墙 DROP / 路由黑洞）")
	case errors.Is(err, syscall.ECONNREFUSED):
		fmt.Printf("   connection refused 耗时 %v\n", cost)
		fmt.Println("   判读：该地址回了 RST，不是黑洞；换一个被 DROP 的地址观察超时")
	default:
		fmt.Printf("   %v 耗时 %v\n", err, cost)
		fmt.Println("   判读：既不是 RST 也不是超时，说明路径上有设备改了行为（如 ICMP 不可达）")
	}

	fmt.Println("\n结论：refused 先查进程与端口，timeout 先查中间设备与路由")
}
