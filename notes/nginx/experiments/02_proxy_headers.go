// # 透传头验证:后端的"用户视角"全靠 proxy_set_header
//
// 对应笔记: 02-反向代理与负载均衡.md「透传头」
//
// 同一个后端代码,三种访问方式,后端看到的世界完全不同:
//
//	直连 8081          : X-Forwarded-Proto 为空, remote_addr = 客户端自己
//	HTTP  走 nginx 8080 : X-Forwarded-Proto=http,  remote_addr = nginx(127.0.0.1)
//	HTTPS 走 nginx 8443 : X-Forwarded-Proto=https, remote_addr = nginx
//	                      ↑ TLS 已在 nginx 终止,后端收到的仍是明文 HTTP!
//
// 结论: 后端感知"用户是否走 HTTPS"的唯一途径是 X-Forwarded-Proto,
//       这正是本模块铁律(TLS 只终止在 nginx)的可验证形态。
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type helloResp struct {
	Backend string `json:"backend"`
	Proto   string `json:"proto"`
	RealIP  string `json:"real_ip"`
	Remote  string `json:"remote"`
}

func fetchJSON(client *http.Client, url string) helloResp {
	var r helloResp
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("  请求 %s 失败: %v\n", url, err)
		return r
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	json.Unmarshal(b, &r)
	return r
}

func Exp2ProxyHeaders() {
	direct := fetchJSON(http.DefaultClient, "http://127.0.0.1:8081/hello")
	viaHTTP := fetchJSON(http.DefaultClient, "http://127.0.0.1:8080/hello")

	tlsCfg := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // 自签证书,-k 的 Go 写法
	viaHTTPS := fetchJSON(&http.Client{Transport: tlsCfg}, "https://127.0.0.1:8443/hello")

	fmt.Printf("  %-22s proto=%-6q remote=%s real_ip=%q\n", "直连 8081", direct.Proto, direct.Remote, direct.RealIP)
	fmt.Printf("  %-22s proto=%-6q remote=%s real_ip=%q\n", "HTTP 走 nginx", viaHTTP.Proto, viaHTTP.Remote, viaHTTP.RealIP)
	fmt.Printf("  %-22s proto=%-6q remote=%s real_ip=%q\n", "HTTPS 走 nginx", viaHTTPS.Proto, viaHTTPS.Remote, viaHTTPS.RealIP)

	fmt.Println()
	fmt.Println("  验证点:")
	fmt.Printf("    - 直连时 proto 为空、remote 是客户端 —— 后端自己没有 TLS 也没有代理信息\n")
	fmt.Printf("    - 走 nginx 后 remote 变成 nginx 地址 —— 必须靠 X-Real-IP 拿真实 IP\n")
	fmt.Printf("    - HTTPS 请求后端 proto=https 但连接是明文 —— TLS 已在 nginx 终止\n\n")
}
