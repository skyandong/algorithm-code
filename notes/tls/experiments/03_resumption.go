// # 实验3：会话恢复 —— TLS 1.3 为什么"感觉"更快
//
// 三种连接方式对比：
//
//	全新 TLS 1.2 握手（2-RTT）
//	全新 TLS 1.3 握手（1-RTT）
//	TLS 1.3 会话恢复（PSK，第二次连同一台服务器）
//
// 观察指标：
//	- ConnectionState.DidResume：是否走了恢复路径
//	- 握手耗时（本机 loopback RTT≈0，差异微小，但 DidResume 一目了然）
//
// 对应笔记《04-会话恢复与0-RTT》。
package main

import (
	"crypto/tls"
	"fmt"
	"time"
)

// ExpResumption 对比全新握手与会话恢复。
func ExpResumption() {
	fmt.Println("=== 实验3：会话恢复 ===")

	root, inter, leaf := genChain()
	addr, stop := startTLSServer(leaf, tls.VersionTLS13, inter)
	defer stop()

	rootPool := newX509Pool(root.Cert)

	newClient := func() *tls.Config {
		return &tls.Config{
			RootCAs:    rootPool,
			ServerName: "www.demo.local",
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
			// 会话缓存：客户端保存服务器发的 session ticket
			ClientSessionCache: tls.NewLRUClientSessionCache(16),
		}
	}

	dialOnce := func(cfg *tls.Config) (*tls.ConnectionState, time.Duration) {
		start := time.Now()
		conn, err := tls.Dial("tcp", addr, cfg)
		if err != nil {
			panic(err)
		}
		// 写一个字节触发握手收尾 + 读回服务端响应（顺便收到 NewSessionTicket）
		conn.Write([]byte("x"))
		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		conn.Read(buf)
		dur := time.Since(start)
		st := conn.ConnectionState()
		conn.Close()
		return &st, dur
	}

	// --- TLS 1.3：全新握手 ---
	cfg13 := newClient()
	st1, d1 := dialOnce(cfg13)

	// --- 同一个 cfg 再次连接：携带 session ticket，走 PSK 恢复 ---
	st2, d2 := dialOnce(cfg13)

	// --- 全新客户端（无缓存）：全新握手 ---
	st3, d3 := dialOnce(newClient())

	fmt.Println("\n--- TLS 1.3 ---")
	fmt.Printf("  第一次连接（全新握手）: resume=%v  cipher=%s  耗时=%v\n",
		st1.DidResume, tls.CipherSuiteName(st1.CipherSuite), d1.Round(time.Microsecond))
	fmt.Printf("  第二次连接（会话恢复）: resume=%v  cipher=%s  耗时=%v  ← PSK 复用\n",
		st2.DidResume, tls.CipherSuiteName(st2.CipherSuite), d2.Round(time.Microsecond))
	fmt.Printf("  新客户端（全新握手）  : resume=%v  cipher=%s  耗时=%v\n",
		st3.DidResume, tls.CipherSuiteName(st3.CipherSuite), d3.Round(time.Microsecond))

	fmt.Println()
	fmt.Println("结论:")
	fmt.Println("  - DidResume=true 表示跳过了完整密钥交换，直接用上次会话派生的 PSK")
	fmt.Println("  - 恢复路径省掉了证书传输和 ECDHE 计算，握手消息更少")
	fmt.Println("  - 这就是浏览器'第二次打开网站更快'的底层原因之一")
	fmt.Println("  - Go 服务端默认支持发 ticket；恢复是客户端缓存驱动的")
}
