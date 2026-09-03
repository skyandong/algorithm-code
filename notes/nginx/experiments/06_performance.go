// # 性能相关:gzip 压缩 + keepalive 的体感差异
//
// 对应笔记: 05-性能调优与排查.md
//
// ① gzip: 同一接口,带不带 Accept-Encoding,响应头 Content-Encoding 不同。
//    conf: gzip on + gzip_types application/json + gzip_min_length 100 + gzip_proxied any
//    后端 /hello 返回 >1KB 的 JSON,足够触发压缩。
//
// ② keepalive: 复用连接 vs 每次新建连接,100 次请求的耗时差。
//    nginx→上游的 keepalive 在 conf 里(upstream keepalive 32 + Connection "")。
//    客户端侧同样有这个开关(Transport.DisableKeepAlives),
//    两端任一处不开,都要为每个请求付一次 TCP 握手成本。
package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

func Exp6Performance() {
	// ① gzip
	req, _ := http.NewRequest("GET", "http://127.0.0.1:8080/hello", nil)
	req.Header.Set("Accept-Encoding", "gzip") // Go http.Get 默认也带,这里显式写清
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		ce := resp.Header.Get("Content-Encoding")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		fmt.Printf("  ① 带 Accept-Encoding: gzip → Content-Encoding=%q (空=没压缩,查 gzip_types/gzip_min_length)\n", ce)
	}

	// ② keepalive 对比
	bench := func(disableKeepAlive bool) time.Duration {
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: disableKeepAlive}}
		start := time.Now()
		for i := 0; i < 100; i++ {
			r, err := client.Get("http://127.0.0.1:8080/hello")
			if err != nil {
				continue
			}
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
		}
		return time.Since(start)
	}

	tReuse := bench(false)
	tFresh := bench(true)
	fmt.Printf("  ② 100 次请求: 复用连接 %v vs 每次新建 %v (差值=反复握手的成本)\n", tReuse, tFresh)
	fmt.Println("     客户端如此,nginx→上游同理: upstream keepalive + proxy_http_version 1.1 + Connection \"\" 三件套")
	fmt.Println()
}
