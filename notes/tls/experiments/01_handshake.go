// # 实验1：观察一次真实的 TLS 握手
//
// 本地起一个 HTTPS 服务器，用客户端连上去，把协商结果全部打出来：
//
//	协议版本（TLS 1.2 还是 1.3）
//	密码套件（CipherSuite）
//	服务器证书链（PeerCertificates）
//	密钥交换方式（ECDHE）
//
// 再对比客户端限制 MaxVersion=TLS1.2 时的差异。
// 对应笔记《01-为什么需要TLS》《02-TLS1.2握手》《03-TLS1.3握手》。
package main

import (
	"crypto/tls"
	"fmt"
	"time"
)

// startTLSServer 在随机端口起一个本地 TLS 服务器，返回地址。
// leaf 是服务器证书，chain... 是随证书一起发送的中间证书链
// （等价于 Nginx 配 fullchain：漏发中间证书客户端就没法验证到根）。
// 服务端行为：握手完成后立即关闭连接（我们只关心握手）。
func startTLSServer(leaf *certKeyPair, serverTLSVersion uint16, chain ...*certKeyPair) (addr string, stop func()) {
	certDERs := [][]byte{leaf.CertDER}
	for _, c := range chain {
		certDERs = append(certDERs, c.CertDER)
	}
	certPair := tls.Certificate{
		Certificate: certDERs,
		PrivateKey:  leaf.Key,
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{certPair},
		MinVersion:   tls.VersionTLS10,
		MaxVersion:   serverTLSVersion,
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			// 读完客户端消息后回写一个字节再关闭：
			// 1. 触发客户端 -> 服务端方向完成完整握手
			// 2. 服务端首次 Write 时会把 NewSessionTicket（TLS 1.3）一并发给客户端
			buf := make([]byte, 1)
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			conn.Read(buf)
			conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
			conn.Write([]byte("y"))
			conn.Close()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// ExpHandshake 观察 TLS 1.3 与 TLS 1.2 握手协商结果的差异。
func ExpHandshake() {
	fmt.Println("=== 实验1：观察 TLS 握手协商 ===")

	root, inter, leaf := genChain()
	addr, stop := startTLSServer(leaf, tls.VersionTLS13, inter)
	defer stop()

	// 客户端信任我们的 Root CA（等价于操作系统预装的根证书库）
	rootPool := newRootPool(root)

	dial := func(maxVer uint16) *tls.ConnectionState {
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			RootCAs:    rootPool,
			ServerName: "www.demo.local",
			MinVersion: tls.VersionTLS10,
			MaxVersion: maxVer,
		})
		if err != nil {
			panic(err)
		}
		defer conn.Close()
		// 写一个字节触发握手收尾，再读回一个字节（确保收到 NewSessionTicket）
		conn.Write([]byte("x"))
		buf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		conn.Read(buf)
		st := conn.ConnectionState()
		return &st
	}

	fmt.Println("\n--- 客户端允许 TLS 1.3 ---")
	st := dial(tls.VersionTLS13)
	printState(st)

	fmt.Println("\n--- 客户端限制最高 TLS 1.2（模拟老客户端/降级） ---")
	st2 := dial(tls.VersionTLS12)
	printState(st2)

	fmt.Println()
	fmt.Println("结论:")
	fmt.Println("  - 版本是双方协商的：客户端和服务器取共同支持的最高版本")
	fmt.Println("  - TLS 1.3 的密码套件名里只有 AEAD+哈希（TLS_AES_128_GCM_SHA256）")
	fmt.Println("  - TLS 1.2 的密码套件名里还有密钥交换和认证（ECDHE_RSA_...）")
	fmt.Println("  - 两种版本都用了 ECDHE（椭圆曲线临时密钥交换）→ 前向安全")
}

// newRootPool 构建只包含 root 的信任库
func newRootPool(root *certKeyPair) *x509CertPool {
	return newX509Pool(root.Cert)
}

func printState(st *tls.ConnectionState) {
	fmt.Printf("  协商版本:        %s\n", tlsVersionName(st.Version))
	fmt.Printf("  密码套件:        %s\n", tls.CipherSuiteName(st.CipherSuite))
	fmt.Printf("  握手完成:        %v\n", st.HandshakeComplete)
	fmt.Printf("  服务器证书链:    %d 张\n", len(st.PeerCertificates))
	for i, c := range st.PeerCertificates {
		role := "叶子"
		if c.IsCA {
			role = "CA"
		}
		fmt.Printf("    [%d] %s (CN=%s, %s~%s, %s)\n",
			i, role, c.Subject.CommonName,
			c.NotBefore.Format("2006-01-02"), c.NotAfter.Format("2006-01-02"),
			c.PublicKeyAlgorithm)
	}
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	}
	return fmt.Sprintf("unknown(0x%04x)", v)
}
