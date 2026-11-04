// # 网络实验统一入口（notes/network）
//
// 全部实验本地自包含（本地监听 + 本地回环），无需外网、无需 root。
//
// 用法:
//
//	go run ./experiments/              # 运行全部
//	go run ./experiments/ handshake    # 实验1：建连成功与两种失败
//	go run ./experiments/ close        # 实验2：半关闭与 TIME_WAIT/CLOSE_WAIT
//	go run ./experiments/ congestion   # 实验3：接收窗口耗尽导致写阻塞
//	go run ./experiments/ http1        # 实验4：keep-alive 复用与失效
//	go run ./experiments/ http2        # 实验5：h2 协商与单连接多路复用
//	go run ./experiments/ quic         # 实验6：四元组 vs Connection ID
//	go run ./experiments/ diagnose     # 实验7：五段耗时拆解
package main

import (
	"fmt"
	"os"
)

type expCase struct {
	name string
	desc string
	run  func()
}

var cases = []expCase{
	{"handshake", "实验1：建连成功、被拒绝（RST）与超时（SYN 被丢）", ExpHandshake},
	{"close", "实验2：半关闭语义与主动/被动关闭的状态差异", ExpClose},
	{"congestion", "实验3：接收窗口耗尽时写调用阻塞（与丢包无关）", ExpCongestion},
	{"http1", "实验4：HTTP/1.1 连接复用与三种失效场景", ExpHTTP1},
	{"http2", "实验5：ALPN 协商 h2，多请求共用一条连接", ExpHTTP2},
	{"quic", "实验6：换端口后按 Connection ID 找回会话（连接迁移）", ExpQUIC},
	{"diagnose", "实验7：DNS/建连/TLS/首字节/传输 五段耗时拆解", ExpDiagnose},
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "all" {
		for _, c := range cases {
			fmt.Printf("\n%s\n", c.desc)
			c.run()
		}
		return
	}
	for _, c := range cases {
		if c.name == os.Args[1] {
			fmt.Printf("\n%s\n", c.desc)
			c.run()
			return
		}
	}
	fmt.Println("用法: go run ./experiments/ [handshake|close|congestion|http1|http2|quic|diagnose|all]")
	for _, c := range cases {
		fmt.Printf("  %-11s %s\n", c.name, c.desc)
	}
}
