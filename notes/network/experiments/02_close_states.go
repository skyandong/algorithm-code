// # 半关闭实验（笔记 02）
//
// 对应笔记：notes/network/02-四次挥手与连接状态.md
//
// 运行：go run ./experiments/ close
package main

import (
	"fmt"
	"io"
	"net"
	"time"
)

// ExpClose 演示 CloseWrite 只关一个方向：发 FIN 后仍可继续读，这是四次挥手的第一半。
func ExpClose() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("监听失败：", err)
		return
	}
	defer ln.Close()

	serverSide := make(chan string, 4)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		b, readErr := io.ReadAll(conn)
		serverSide <- fmt.Sprintf("   服务端：读到 %q（err=%v），Read 返回 EOF —— 对方的 FIN 到了（进入 CLOSE_WAIT）", b, readErr)
		time.Sleep(30 * time.Millisecond)
		if _, err := conn.Write([]byte("response-after-half-close")); err != nil {
			serverSide <- fmt.Sprintf("   服务端：写失败 %v", err)
			return
		}
		serverSide <- "   服务端：半关闭后仍向对方写了 25 字节 —— 另一方向还没关"
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		fmt.Println("连接失败：", err)
		return
	}
	defer conn.Close()

	fmt.Println("客户端：发送 \"request\"")
	if _, err := conn.Write([]byte("request")); err != nil {
		fmt.Println("客户端写失败：", err)
		return
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		fmt.Println("不是 TCP 连接，跳过")
		return
	}
	if err := tcpConn.CloseWrite(); err != nil {
		fmt.Println("CloseWrite 失败：", err)
		return
	}
	fmt.Println("客户端：CloseWrite() —— 只关写方向（发 FIN），读方向仍然可用")

	for i := 0; i < 2; i++ {
		fmt.Println(<-serverSide)
	}

	buf := make([]byte, 64)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		fmt.Println("设置读超时失败：", err)
		return
	}
	n, err := conn.Read(buf)
	fmt.Printf("客户端：读到 %q（err=%v）—— 半关闭下依然收到了响应\n", buf[:n], err)

	fmt.Println("\n状态说明：")
	fmt.Println("  主动关闭方（本实验的客户端）：先发 FIN，最终进 TIME_WAIT 并等待 2MSL")
	fmt.Println("  被动关闭方（本实验的服务端）：收到 FIN 进 CLOSE_WAIT，直到应用 Close 才发自己的 FIN")
	fmt.Println("  CLOSE_WAIT 只增不减 = 应用漏了 Close，是代码 bug；TIME_WAIT 是正常回收过程")
}
