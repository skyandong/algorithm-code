// # 五段耗时拆解实验（笔记 07）
//
// 对应笔记：notes/network/07-排查实战.md
//
// 运行：go run ./experiments/ diagnose
package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"syscall"
	"time"
)

// ExpDiagnose 用 httptrace 在客户端复刻 curl -w 的五段计时：
// DNS 解析 → TCP 建连 → TLS 握手 → 首字节 → 内容传输。
func ExpDiagnose() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 8<<10)
		w.Write(body)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{}}
	fmt.Println("① 第一次请求（全新连接，可能包含 DNS 与建连）")
	reportTrace(client, srv.URL)

	fmt.Println("\n② 第二次请求（同一 client，预期复用连接）")
	reportTrace(client, srv.URL)

	fmt.Println("\n③ 失败的两段：拒绝 vs 超时")
	start := time.Now()
	_, err := net.DialTimeout("tcp", "127.0.0.1:1", 500*time.Millisecond)
	fmt.Printf("   拒绝：%-20v 耗时 %v（收到 RST，立刻返回）\n", err, time.Since(start).Round(time.Millisecond))

	start = time.Now()
	_, err = net.DialTimeout("tcp", "10.255.255.1:9", 700*time.Millisecond)
	cost := time.Since(start).Round(time.Millisecond)
	switch {
	case err == nil:
		fmt.Printf("   本机对 10.255.255.1:9 的 dial 竟然成功（耗时 %v）——网络里有代理/网关代答，\n", cost)
		fmt.Println("   该环境演示不出 timeout；直连环境下会是 i/o timeout")
	case os.IsTimeout(err):
		fmt.Printf("   超时：i/o timeout 耗时 %v（SYN 无应答，等到 deadline）\n", cost)
		fmt.Println("   注：timeout 说明 SYN 没有应答，归因中间设备而不是应用")
	case errors.Is(err, syscall.ECONNREFUSED):
		fmt.Printf("   拒绝：%v 耗时 %v\n", err, cost)
		fmt.Println("   注：该地址回了 RST，不是黑洞；换一个被防火墙 DROP 的地址观察超时")
	default:
		fmt.Printf("   其它：%v 耗时 %v\n", err, cost)
	}

	fmt.Println()
	fmt.Println("判读口诀：")
	fmt.Println("  connect 段慢 → 网络/内核（防火墙、路由、backlog）")
	fmt.Println("  ttfb 段慢    → 应用（连接池、下游依赖、锁、GC）")
	fmt.Println("  total−ttfb 慢 → 带宽/窗口（算 BDP，看 ss -ti 的 cwnd 与 retrans）")
}

// reportTrace 发一次请求并打印五段耗时。时间戳为零表示该阶段没有发生。
func reportTrace(client *http.Client, url string) {
	var (
		dnsStart, dnsDone         time.Time
		connStart, connDone       time.Time
		tlsStart, tlsDone         time.Time
		gotConn, wrote, firstByte time.Time
		reused                    bool
	)
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart:         func(string, string) { connStart = time.Now() },
		ConnectDone:          func(string, string, error) { connDone = time.Now() },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsDone = time.Now() },
		GotConn:              func(info httptrace.GotConnInfo) { gotConn = time.Now(); reused = info.Reused },
		WroteRequest:         func(httptrace.WroteRequestInfo) { wrote = time.Now() },
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("   构造请求失败：", err)
		return
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("   请求失败：", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	total := time.Since(start)

	fmt.Printf("   DNS:       %s\n", seg(dnsStart, dnsDone))
	fmt.Printf("   connect:   %s\n", seg(connStart, connDone))
	fmt.Printf("   tls:       %s\n", seg(tlsStart, tlsDone))
	fmt.Printf("   ttfb:      %s\n", seg(wrote, firstByte))
	fmt.Printf("   传输:      %s\n", seg(firstByte, start.Add(total)))
	fmt.Printf("   总计:      %v（复用连接 %v，协议 %s）\n", total.Round(time.Microsecond), reused, resp.Proto)
	if gotConn.IsZero() {
		fmt.Println("   注：GotConn 未触发，请求可能在 DNS 或建连阶段就失败了")
	}
}

// seg 计算两个时间戳之间的耗时；任一为零表示该阶段未发生。
func seg(from, to time.Time) string {
	if from.IsZero() || to.IsZero() {
		return "—（未发生，例如 URL 是 IP 字面量时没有 DNS）"
	}
	return to.Sub(from).Round(time.Microsecond).String()
}
