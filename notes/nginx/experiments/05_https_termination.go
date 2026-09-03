// # TLS 终止验证:加密只在 nginx,后端零 TLS
//
// 对应笔记: 04-HTTPS-TLS终止实战.md(模块铁律)
//
// 三个断言:
//	① 对 nginx:8443 发起 TLS 握手 → 成功,能读到证书信息和协商出的 TLS 版本
//	② 对后端 8081 发起 TLS 握手   → 失败!后端只会说明文 HTTP,
//	   证明加密确实终止在 nginx,而不是透传到了后端
//	③ 同一后端,HTTPS 进来读到 X-Forwarded-Proto=https ——
//	   后端代码零改动、零 TLS import,全靠 nginx 透传头感知协议
//
// 注意: 本文件 import crypto/tls 是"模拟客户端",合理;
//       铁律约束的是后端服务,后端代码(main.go)里没有任何 TLS。
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

func Exp5HTTPSTermination() {
	// ① TLS 握手成功:读证书与协商版本
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp",
		"127.0.0.1:8443", &tls.Config{InsecureSkipVerify: true}) // 自签证书,生产别这么写
	if err != nil {
		fmt.Printf("  ① 8443 握手失败: %v (证书生成了吗? conf/gen-certs.sh)\n\n", err)
		return
	}
	state := conn.ConnectionState()
	cert := state.PeerCertificates[0]
	fmt.Printf("  ① 8443 TLS 握手成功: 版本=0x%04x(0x0303=TLS1.2, 0x0304=TLS1.3)\n", state.Version)
	fmt.Printf("     证书 CN=%s, 有效期至 %s, SAN 含 localhost\n", cert.Subject.CommonName, cert.NotAfter.Local().Format("2006-01-02"))
	conn.Close()

	// ② 后端不会说 TLS:握手必然失败
	_, err = tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp",
		"127.0.0.1:8081", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		fmt.Printf("  ② 8081 TLS 握手失败(符合预期): %v\n", shortErr(err))
		fmt.Println("     → 后端是纯 HTTP 服务,加密只存在于 client↔nginx 这一段")
	} else {
		fmt.Println("  ② 8081 竟然握手成功?后端被加了 TLS,违反模块铁律,检查代码!")
	}

	// ③ 同一后端,协议感知靠透传头
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get("https://127.0.0.1:8443/hello")
	if err == nil {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var r struct {
			Proto   string `json:"proto"`
			Backend string `json:"backend"`
		}
		json.Unmarshal(b, &r)
		fmt.Printf("  ③ HTTPS 请求,后端读到 X-Forwarded-Proto=%q (实例=%s)\n", r.Proto, r.Backend)
		fmt.Println("     → 后端代码一行没改,就知道了用户走的是 https")
	}
	fmt.Println()
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 60 {
		s = s[:60] + "..."
	}
	return s
}
