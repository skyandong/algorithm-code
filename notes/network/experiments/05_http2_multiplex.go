// # HTTP/2 多路复用实验（笔记 05）
//
// 对应笔记：notes/network/05-HTTP2多路复用.md
//
// 运行：go run ./experiments/ http2
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
)

// ExpHTTP2 对比同一批并发请求在 h2 与 h1 下占用多少条 TCP 连接。
func ExpHTTP2() {
	// 注意：httptest.NewTLSServer 默认不启用 h2，必须用 EnableHTTP2 显式打开
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	srv.EnableHTTP2 = true
	// 收尾时关闭空闲连接会触发 "use of closed network connection" 的 TLS 报错日志，压掉噪音
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()

	base, ok := srv.Client().Transport.(*http.Transport)
	if !ok {
		fmt.Println("httptest 客户端的 Transport 不是 *http.Transport，跳过")
		return
	}
	tlsCfg := base.TLSClientConfig.Clone()

	const n = 8

	fmt.Println("① 允许 h2（ForceAttemptHTTP2=true，走 ALPN 协商，服务端 EnableHTTP2=true）")
	proto2, conns2, err := runConcurrent(&http.Transport{
		TLSClientConfig:   tlsCfg.Clone(),
		ForceAttemptHTTP2: true,
	}, srv.URL, n)
	if err != nil {
		fmt.Println("   请求失败：", err)
		return
	}
	fmt.Printf("   %d 个并发请求 → 协议 %s，实际用了 %d 条 TCP 连接\n", n, proto2, conns2)
	fmt.Println("   判读：多路复用生效，一条连接上并存多条流，只付一次握手与慢启动")

	fmt.Println("\n② 关掉 h2（TLSNextProto 置空 + ForceAttemptHTTP2=false），退回 HTTP/1.1")
	proto1, conns1, err := runConcurrent(&http.Transport{
		TLSClientConfig:   tlsCfg.Clone(),
		ForceAttemptHTTP2: false,
		TLSNextProto:      map[string]func(string, *tls.Conn) http.RoundTripper{},
	}, srv.URL, n)
	if err != nil {
		fmt.Println("   请求失败：", err)
		return
	}
	fmt.Printf("   %d 个并发请求 → 协议 %s，实际用了 %d 条 TCP 连接\n", n, proto1, conns1)
	fmt.Println("   判读：1.1 只能靠开多条连接换并发，每条都要各自握手")

	fmt.Println()
	fmt.Println("结论：")
	fmt.Println("  h2 把并发压到一条连接上，代价是所有流共享同一条 TCP 的字节序——")
	fmt.Println("  一旦丢包，内核要等重传补齐，全部流一起卡住（TCP 层队头阻塞）。")
	fmt.Println("  这正是 HTTP/3 换到 QUIC 的直接动机。")
}

// runConcurrent 并发发起 n 个请求，返回协议版本与去重后的连接数。
func runConcurrent(tr *http.Transport, url string, n int) (string, int, error) {
	client := &http.Client{Transport: tr}
	defer client.CloseIdleConnections()

	var (
		mu    sync.Mutex
		conns = map[net.Conn]struct{}{}
		proto string
	)
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			trace := &httptrace.ClientTrace{
				GotConn: func(info httptrace.GotConnInfo) {
					mu.Lock()
					conns[info.Conn] = struct{}{}
					mu.Unlock()
				},
			}
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				errCh <- err
				return
			}
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
			resp, err := client.Do(req)
			if err != nil {
				errCh <- err
				return
			}
			mu.Lock()
			proto = resp.Proto
			mu.Unlock()
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		return "", 0, err
	}
	return proto, len(conns), nil
}
