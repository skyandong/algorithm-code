package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	// 检查证书
	if _, err := os.Stat("localhost.pem"); os.IsNotExist(err) {
		log.Fatal("证书不存在，先启动 server 生成证书")
	}

	// HTTP/3 客户端（使用 http3.RoundTripper）
	client := &http.Client{
		Transport: &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // 本地测试跳过证书验证
			},
		},
	}
	defer client.CloseIdleConnections()

	// 请求根路径
	resp, err := client.Get("https://localhost:4433/")
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Printf("✅ / 响应: %s", body)
	fmt.Printf("   协议: %s\n\n", resp.Proto)

	// 请求 /time
	resp2, _ := client.Get("https://localhost:4433/time")
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("✅ /time 响应: %s\n", body2)
	fmt.Printf("   协议: %s\n", resp2.Proto)
}
