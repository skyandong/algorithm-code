// # 连接迁移实验（笔记 06）
//
// 对应笔记：notes/network/06-HTTP3与QUIC.md
//
// 运行：go run ./experiments/ quic
//
// 说明：这是一个最小化的概念验证，不是 QUIC 实现——只演示 QUIC 最核心的设计差异：
// 会话按 Connection ID 标识，而不是按四元组标识。全程只用到 UDP。
package main

import (
	"fmt"
	"net"
	"sync"
	"time"
)

const connIDLen = 16

// connIDSession 是按 Connection ID 索引的会话（QUIC 的做法）。
type connIDSession struct{ hits int }

// tupleSession 是按「客户端地址:端口」索引的会话（TCP 的做法）。
type tupleSession struct{ hits int }

// ExpQUIC 演示客户端换本地端口后，两种识别方式的差异。
func ExpQUIC() {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("UDP 监听失败：", err)
		return
	}
	defer pc.Close()
	srvAddr, ok := pc.LocalAddr().(*net.UDPAddr)
	if !ok {
		fmt.Println("监听地址类型异常，跳过")
		return
	}

	var (
		mu          sync.Mutex
		byConnID    = map[string]*connIDSession{}
		byTuple     = map[string]*tupleSession{}
		lastConnIDH int
		lastTupleH  int
	)

	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n <= connIDLen {
				continue
			}
			cid := string(buf[:connIDLen])
			payload := string(buf[connIDLen:n])

			mu.Lock()
			cs, ok := byConnID[cid]
			if !ok {
				cs = &connIDSession{}
				byConnID[cid] = cs
			}
			cs.hits++

			ts, ok := byTuple[addr.String()]
			if !ok {
				ts = &tupleSession{}
				byTuple[addr.String()] = ts
			}
			ts.hits++

			lastConnIDH, lastTupleH = cs.hits, ts.hits
			mu.Unlock()

			reply := fmt.Sprintf("ack payload=%q connID会话累计=%d 四元组会话累计=%d", payload, lastConnIDH, lastTupleH)
			pc.WriteTo([]byte(reply), addr)
		}
	}()

	connID := "0123456789abcdef"

	fmt.Println("① 第一次发送（本地端口 A）")
	if msg, ok := sendUDP(srvAddr, 0, connID, "hello"); ok {
		fmt.Println("   ", msg)
	}

	fmt.Println("\n② 模拟网络切换：换一个本地端口（相当于 WiFi → 4G），Connection ID 不变")
	if msg, ok := sendUDP(srvAddr, 0, connID, "hello-again"); ok {
		fmt.Println("   ", msg)
	}

	fmt.Println()
	fmt.Println("判读：")
	fmt.Println("  Connection ID 视角：两次属于同一个会话，累计计数为 2 —— 会话跨路径存活，无需重新握手")
	fmt.Println("  四元组视角：客户端地址变了就是新连接，第二次只能从 1 开始 —— 在 TCP 上意味着重连 + 重新握手")
	fmt.Println("  这就是 QUIC 连接迁移的全部含义：身份与路径解耦。")
}

// sendUDP 用一个全新的本地端口发一包（port=0 由内核分配），并等回包。
func sendUDP(srvAddr *net.UDPAddr, port int, connID, payload string) (string, bool) {
	conn, err := net.DialUDP("udp",
		&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}, srvAddr)
	if err != nil {
		fmt.Println("   发送失败：", err)
		return "", false
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		fmt.Println("   设置超时失败：", err)
		return "", false
	}
	packet := append([]byte(connID), payload...)
	if _, err := conn.Write(packet); err != nil {
		fmt.Println("   写失败：", err)
		return "", false
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("   回包读取失败：", err)
		return "", false
	}
	return fmt.Sprintf("本地 %s → 服务端回 %q", conn.LocalAddr(), string(buf[:n])), true
}
