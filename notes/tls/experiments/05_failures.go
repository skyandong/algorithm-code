// # 实验5：握手失败的各种姿势（对照排查手册）
//
// 刻意制造四种失败，观察 Go/crypto/tls 报出的错误——
// 这些错误对应浏览器里的各种证书告警，生产排查时按图索骥。
//
//	1. 不信任的 CA      → x509: certificate signed by unknown authority
//	2. 域名不匹配       → x509: certificate is valid for ..., not ...
//	3. 证书过期         → x509: certificate has expired or is not yet valid
//	4. 客户端只说 TLS1.0, 服务器要求 ≥1.2 → protocol version mismatch
//
// 对应笔记《07-实战排查》。
package main

import (
	"crypto/tls"
	"fmt"
	"time"
)

// ExpFailures 制造并观察握手失败。
func ExpFailures() {
	fmt.Println("=== 实验5：握手失败的各种姿势 ===")

	root, inter, leaf := genChain()
	rootPool := newX509Pool(root.Cert)

	try := func(name string, f func() error) {
		if err := f(); err != nil {
			fmt.Printf("  %-14s ✗ %v\n", name, err)
		} else {
			fmt.Printf("  %-14s ✓ 意外成功\n", name)
		}
	}

	// 场景1：客户端不信任签发的 CA（比如自签名证书）
	try("不受信任CA", func() error {
		addr, stop := startTLSServer(leaf, tls.VersionTLS13, inter)
		defer stop()
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: "www.demo.local",
			RootCAs:    newX509Pool(genChainSelfSigned().Cert), // 信任另一个 CA
			MinVersion: tls.VersionTLS12,
		})
		if err == nil {
			conn.Close()
		}
		return err
	})

	// 场景2：域名不匹配
	try("域名不匹配", func() error {
		addr, stop := startTLSServer(leaf, tls.VersionTLS13, inter)
		defer stop()
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: "wrong.name.com",
			RootCAs:    rootPool,
			MinVersion: tls.VersionTLS12,
		})
		if err == nil {
			conn.Close()
		}
		return err
	})

	// 场景3：证书过期
	try("证书过期", func() error {
		expired := expiredLeaf(inter)
		addr, stop := startTLSServer(expired, tls.VersionTLS13, inter)
		defer stop()
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: "expired.demo.local",
			RootCAs:    rootPool,
			MinVersion: tls.VersionTLS12,
		})
		if err == nil {
			conn.Close()
		}
		return err
	})

	// 场景4：版本协商失败（客户端只会 TLS 1.0，服务器要求 1.2+）
	try("版本不匹配", func() error {
		// 专门起一个 MinVersion=TLS1.2 的服务器（startTLSServer 允许到 1.0）
		ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			Certificates: []tls.Certificate{{
				Certificate: [][]byte{leaf.CertDER, inter.CertDER},
				PrivateKey:  leaf.Key,
			}},
			MinVersion: tls.VersionTLS12, // 服务器拒绝 1.0/1.1
		})
		if err != nil {
			return err
		}
		defer ln.Close()
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				buf := make([]byte, 1)
				conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				conn.Read(buf)
				conn.Close()
			}
		}()
		conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
			ServerName: "www.demo.local",
			RootCAs:    rootPool,
			MinVersion: tls.VersionTLS10,
			MaxVersion: tls.VersionTLS10, // 客户端只会 TLS 1.0
		})
		if err == nil {
			conn.Close()
		}
		return err
	})

	fmt.Println()
	fmt.Println("结论:")
	fmt.Println("  - 错误信息直接指明原因：unknown authority / valid for / expired / version")
	fmt.Println("  - 生产环境同样的错误：先 openssl s_client -connect host:443 -showcerts 看链")
	fmt.Println("  - 'certificate signed by unknown authority' 十有八九是缺中间证书或内网CA未安装")
}

// genChainSelfSigned 一个无关的 CA（用于"不信任"场景）
func genChainSelfSigned() *certKeyPair {
	return genCA("Other CA", 365)
}
