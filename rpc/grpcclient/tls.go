package main

// TLS 客户端凭证：正规写法 = 把自签证书加进信任池 + 校验服务端域名（SAN）。
// 千万别图省事全用 InsecureSkipVerify —— 那等于没防中间人。
//
// 证书复用 notes/nginx 的自签证书（模块间衔接）：gen-certs.sh 生成。

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"google.golang.org/grpc/credentials"
)

const (
	certFile = "../../notes/nginx/conf/certs/server.crt"
	keyFile  = "../../notes/nginx/conf/certs/server.key" // 客户端用不到 key，留着对照
)

func tlsCreds() credentials.TransportCredentials {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		return nil
	}
	return credentials.NewTLS(&tls.Config{
		RootCAs:    pool,              // 信任这份自签证书
		ServerName: "localhost",       // 校验证书 SAN 里的域名，不匹配即握手失败
		MinVersion: tls.VersionTLS12,  // 1.0/1.1 有已知漏洞
	})
}
