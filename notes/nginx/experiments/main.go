package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// 两个纯 HTTP 后端（8081/8082）。
//
// 设计基线：后端代码里没有任何 TLS —— 只 ListenAndServe，绝不 ListenAndServeTLS，
// 不 import crypto/tls，不加载证书。它甚至不知道客户端走的是不是 HTTPS，
// 只能通过 nginx 透传的 X-Forwarded-Proto 头感知（见 04 篇）。

var helloPayload = fmt.Sprintf(`{"desc":"payload 用于 gzip 实验，填到 1KB 以上","pad":"%s"}`,
	string(make([]byte, 1200)))

func backendHandler(id int) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"backend": fmt.Sprintf("backend-%d", id), // 负载均衡实验用它区分实例
			"path":    r.URL.Path,
			"proto":   r.Header.Get("X-Forwarded-Proto"), // 走 nginx 才有；直连为空
			"real_ip": r.Header.Get("X-Real-IP"),
			"xff":     r.Header.Get("X-Forwarded-For"),
			"remote":  r.RemoteAddr, // 直连=客户端地址；反代=nginx 地址
			"payload": helloPayload, // 凑体积给 gzip 实验
		})
	})

	// 01 实验：location 兜底验证 —— 任何打到后端的路径都说明 nginx 没匹配到静态 return
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "match:backend path=%s\n", r.URL.Path)
	})

	return mux
}

func startBackend(addr string, id int) {
	srv := &http.Server{Addr: addr, Handler: backendHandler(id)}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("后端 %s 启动失败: %v", addr, err)
		}
	}()
}

func main() {
	startBackend("127.0.0.1:8081", 1)
	startBackend("127.0.0.1:8082", 2)

	fmt.Println("两个纯 HTTP 后端已启动: 127.0.0.1:8081 / 127.0.0.1:8082（零 TLS）")
	fmt.Println("前置条件: nginx 已按 conf/nginx.conf 启动（8080/8083/8443）")
	fmt.Println()

	fmt.Println("========== 第一节:location 匹配优先级 ==========")
	Exp1LocationMatch()

	fmt.Println("========== 第二节:透传头(反代视角) ==========")
	Exp2ProxyHeaders()

	fmt.Println("========== 第三节:负载均衡分布 ==========")
	Exp3LoadBalance()

	fmt.Println("========== 第四节:限流(漏桶) ==========")
	Exp4RateLimit()

	fmt.Println("========== 第五节:TLS 终止 ==========")
	Exp5HTTPSTermination()

	fmt.Println("========== 第六节:性能(gzip/keepalive) ==========")
	Exp6Performance()
}
