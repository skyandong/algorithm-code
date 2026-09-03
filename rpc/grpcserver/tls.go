package main

// TLS 凭证：服务端加密监听（衔接 notes/tls 与 notes/nginx/04）。
//
// 与 nginx TLS 终止的关系：nginx 场景里 TLS 在边缘代理终止，后端纯 HTTP；
// 而 gRPC 是点对点 RPC，没有边缘代理时**服务端自己持有证书**是正常形态 ——
// 两者不矛盾：nginx 铁律约束的是"web 服务"，RPC 服务间加密属于另一层。
//
// 复用 notes/nginx/conf/gen-certs.sh 生成的自签证书（SAN 含 localhost）。
// 证书不存在时跳过 8889，不影响主流程。

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"grpcserver/idl/hello"
)

const (
	certFile = "../../notes/nginx/conf/certs/server.crt"
	keyFile  = "../../notes/nginx/conf/certs/server.key"
)

// startTLSListener 起一个 TLS 版的 gRPC（同一套业务实现，端口 8889）
func startTLSListener() {
	if _, err := os.Stat(certFile); err != nil {
		log.Println("未找到自签证书，跳过 TLS 监听（生成方式: notes/nginx/conf/gen-certs.sh）")
		return
	}

	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		log.Println("加载证书失败，跳过 TLS 监听:", err)
		return
	}

	lis, err := net.Listen("tcp", ":8889")
	if err != nil {
		log.Println("8889 被占用，跳过 TLS 监听:", err)
		return
	}

	s := grpc.NewServer(grpc.Creds(creds)) // 和明文 server 唯一的区别就是 Creds 选项
	hello.RegisterHelloServiceServer(s, &server{})

	log.Println("gRPC TLS 服务运行在 :8889（证书复用 notes/nginx 的自签证书）")
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()
}

// 供客户端用的"正规"TLS 配置：信任自签 CA + 校验域名（比 InsecureSkipVerify 正确）
func clientTLSCreds() credentials.TransportCredentials {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		return nil
	}
	return credentials.NewTLS(&tls.Config{
		RootCAs:    pool,
		ServerName: "localhost", // 校验证书 SAN，失败即握手失败
		MinVersion: tls.VersionTLS12,
	})
}
