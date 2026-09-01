// # TLS 实验统一入口
//
// 用法:
//
//	go run ./experiments/            # 运行全部
//	go run ./experiments/ handshake  # 实验1：观察握手协商
//	go run ./experiments/ certs      # 实验2：证书链验证
//	go run ./experiments/ resume     # 实验3：会话恢复
//	go run ./experiments/ mtls       # 实验4：mTLS 双向认证
//	go run ./experiments/ fail       # 实验5：握手失败排查
//
// 全部实验自包含（本地生成证书、本地起服务），无需网络。
package main

import (
	"crypto/x509"
	"fmt"
	"os"
)

type tlsCase struct {
	name string
	desc string
	run  func()
}

var cases = []tlsCase{
	{"handshake", "实验1：观察 TLS 1.3 / 1.2 握手协商结果", ExpHandshake},
	{"certs", "实验2：证书链验证（成功与五种失败）", ExpCertChain},
	{"resume", "实验3：会话恢复（全新握手 vs PSK 恢复）", ExpResumption},
	{"mtls", "实验4：mTLS 双向认证", ExpMTLS},
	{"fail", "实验5：握手失败的各种姿势", ExpFailures},
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "all" {
		for _, c := range cases {
			fmt.Printf("\n%s\n", c.desc)
			c.run()
		}
		return
	}
	exp := os.Args[1]
	for _, c := range cases {
		if c.name == exp {
			fmt.Printf("\n%s\n", c.desc)
			c.run()
			return
		}
	}
	usage()
}

func usage() {
	fmt.Println("用法: go run ./experiments/ [handshake|certs|resume|mtls|fail|all]")
	for _, c := range cases {
		fmt.Printf("  %-10s %s\n", c.name, c.desc)
	}
}

// x509CertPool 简写别名
type x509CertPool = x509.CertPool

// newX509Pool 构建包含若干证书的证书池（信任库）
func newX509Pool(certs ...*x509.Certificate) *x509CertPool {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	return pool
}
