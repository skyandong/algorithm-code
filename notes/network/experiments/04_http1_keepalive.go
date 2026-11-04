// # HTTP/1.1 连接复用实验（笔记 04）
//
// 对应笔记：notes/network/04-HTTP1.1与连接复用.md
//
// 运行：go run ./experiments/ http1
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
)

// ExpHTTP1 用 httptrace 的 GotConn.Reused 观察三种情况下连接到底有没有被复用。
func ExpHTTP1() {
	big := strings.Repeat("x", 100<<10) // 100KB，足够让"不读 body"真的不可复用
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			w.Header().Set("Content-Length", fmt.Sprint(len(big)))
			io.WriteString(w, big)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	// 场景 A：读完 body —— 连接回到空闲池，第二次复用
	trA := &http.Transport{}
	clientA := &http.Client{Transport: trA}
	a1 := doReq(clientA, srv.URL+"/", true)
	a2 := doReq(clientA, srv.URL+"/", true)
	fmt.Printf("A 顺序两次、每次都读干 body：复用情况 [%v %v]\n", a1, a2)
	fmt.Println("   判读：第二次 Reused=true，省掉一次握手与慢启动")

	// 场景 B：大 body 不读就 Close —— 连接不可复用
	trB := &http.Transport{}
	clientB := &http.Client{Transport: trB}
	b1 := doReq(clientB, srv.URL+"/big", true)
	b2 := doReq(clientB, srv.URL+"/big", false)
	b3 := doReq(clientB, srv.URL+"/", true)
	fmt.Printf("B 预热 → 大 body 不读就 Close → 再请求：复用情况 [%v %v %v]\n", b1, b2, b3)
	fmt.Println("   判读：第三次 Reused=false，说明中间那条连接被丢弃了（body 没读干，无法界定边界，只能关）")

	// 场景 C：关掉 keep-alive
	trC := &http.Transport{DisableKeepAlives: true}
	clientC := &http.Client{Transport: trC}
	c1 := doReq(clientC, srv.URL+"/", true)
	c2 := doReq(clientC, srv.URL+"/", true)
	fmt.Printf("C DisableKeepAlives=true：复用情况 [%v %v]\n", c1, c2)
	fmt.Println("   判读：两次都是新连接，等于回到 HTTP/1.0 的行为")

	fmt.Println()
	fmt.Println("工程结论：")
	fmt.Println("  1. 用完必须读干 body（io.Copy(io.Discard, resp.Body)）再 Close")
	fmt.Println("  2. Transport 的 MaxIdleConnsPerHost 默认只有 2，高并发下等于没池子")
	fmt.Println("  3. 线上表现为「TIME_WAIT 暴涨 + 延迟抖动」，但根因是应用没读干 body")
}

// doReq 发起一次请求，返回该请求是否复用了已有连接。
// drain=false 时故意不读 body 直接 Close，模拟常见的资源泄漏写法。
func doReq(client *http.Client, url string, drain bool) bool {
	reused := false
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("构造请求失败：", err)
		return false
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("请求失败：", err)
		return false
	}
	if drain {
		io.Copy(io.Discard, resp.Body)
	}
	resp.Body.Close()
	return reused
}
