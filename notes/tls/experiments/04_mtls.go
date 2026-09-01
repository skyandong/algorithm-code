// # 实验4：mTLS 双向认证
//
// 普通 TLS：客户端验证服务器证书（防假服务器）
// mTLS：   服务器也验证客户端证书（防假客户端）
//
// 演示：
//	1. 客户端带证书 → 服务器验证通过，并打印出客户端身份
//	2. 客户端不带证书 → 服务器在握手阶段拒绝
//
// 注意一个 TLS 1.3 细节：客户端握手可能先于服务器验证完成，
// 所以失败要等第一次真正读写时才暴露（实验里 Write 一次触发）。
//
// mTLS 是服务网格（Istio）、零信任网络的身份底座。
// 对应笔记《08-mTLS》。
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"time"
)

// ExpMTLS 演示双向认证的成功与拒绝。
func ExpMTLS() {
	fmt.Println("=== 实验4：mTLS 双向认证 ===")

	root, inter, leaf := genChain()
	// 给"客户端"这个身份签一张证书（由同一个 Root 签发，服务器才能验证）
	clientKey := genClientCert(root)
	rootPool := newX509Pool(root.Cert)

	// 服务器证书：叶子 + 中间证书一起发（fullchain）
	serverCert := tls.Certificate{
		Certificate: [][]byte{leaf.CertDER, inter.CertDER},
		PrivateKey:  leaf.Key,
	}
	addr, stop, seen := startMTLSServer(serverCert, rootPool)
	defer stop()

	fmt.Println("\n--- 场景1：客户端带证书（合法身份） ---")
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		RootCAs:    rootPool,
		ServerName: "www.demo.local",
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{clientKey.CertDER},
			PrivateKey:  clientKey.Key,
		}},
	})
	if err != nil {
		fmt.Printf("  ✗ 握手失败: %v\n", err)
	} else {
		// 写一个字节，让服务器完成"验证客户端证书"这一步
		conn.Write([]byte("x"))
		conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 1)
		if _, rerr := conn.Read(buf); rerr != nil {
			fmt.Printf("  ✗ 服务器拒绝: %v\n", rerr)
		} else {
			fmt.Printf("  ✓ 握手成功, 服务器看到的客户端身份: CN=%s\n", <-seen)
		}
		conn.Close()
	}

	fmt.Println("\n--- 场景2：客户端不带证书（匿名访问） ---")
	conn2, err := tls.Dial("tcp", addr, &tls.Config{
		RootCAs:    rootPool,
		ServerName: "www.demo.local",
	})
	if err != nil {
		fmt.Printf("  ✗ 握手失败: %v\n", err)
	} else {
		// TLS 1.3 下握手可能先"假成功"，真正读写时才收到服务器的拒绝 alert
		conn2.SetDeadline(time.Now().Add(2 * time.Second))
		_, werr := conn2.Write([]byte("x"))
		buf := make([]byte, 1)
		_, rerr := conn2.Read(buf)
		if werr != nil || rerr != nil {
			err := werr
			if err == nil {
				err = rerr
			}
			fmt.Printf("  ✗ 被服务器拒绝: %v\n", err)
			fmt.Println("  ← RequireAndVerifyClientCert：没有客户端证书，服务器发送 alert")
		} else {
			fmt.Println("  ✓ 意外成功（不该发生）")
		}
		conn2.Close()
	}

	fmt.Println()
	fmt.Println("结论:")
	fmt.Println("  - mTLS = 服务器把 ClientAuth 设为 RequireAndVerifyClientCert")
	fmt.Println("  - 客户端证书由同一 CA 签发 → 服务器用它验证'连接的是谁'")
	fmt.Println("  - 证书即身份：Istio/零信任网络里，服务身份就是一张短周期客户端证书")
}

// startMTLSServer 起一个强制要求客户端证书的服务器。
// seen 是服务器视角看到的客户端证书 CN（每次成功握手发送一次）。
func startMTLSServer(cert tls.Certificate, clientRoots *x509CertPool) (addr string, stop func(), seen chan string) {
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientRoots, // 服务器用来验证客户端证书的 CA
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		panic(err)
	}
	seen = make(chan string, 8)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conn := c.(*tls.Conn)
			// 读一个字节触发并完成服务端视角的握手（验证客户端证书在此时发生）
			buf := make([]byte, 1)
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, err := conn.Read(buf); err == nil {
				if st := conn.ConnectionState(); len(st.PeerCertificates) > 0 {
					seen <- st.PeerCertificates[0].Subject.CommonName
				}
				conn.Write([]byte("y"))
			}
			conn.Close()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }, seen
}

// genClientCert 签发一张客户端证书。
// 注意 ExtKeyUsage 必须是 ClientAuth——用服务端证书(ServerAuth)当客户端证书，
// 服务器验证时会报 "bad certificate"（密钥用途不匹配）。
func genClientCert(root *certKeyPair) *certKeyPair {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "service-a"},
		DNSNames:     []string{"service-a"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 90),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, root.Cert, key.Public(), root.Key)
	cert, _ := x509.ParseCertificate(der)
	return &certKeyPair{Cert: cert, Key: key, CertDER: der}
}
