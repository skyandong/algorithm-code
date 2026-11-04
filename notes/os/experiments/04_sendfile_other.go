//go:build !linux

// # sendfile 说明（笔记 04，非 Linux 平台）
//
// 对应笔记：notes/os/04-零拷贝与页缓存.md
package main

import "fmt"

// runSendfileDemo 在非 Linux 平台只说明差异：macOS 也有 sendfile，
// 但签名与 Linux 不同（多一个 hdtr 与 flags 参数），Go 的 syscall 包未暴露。
func runSendfileDemo(path string) {
	_ = path
	fmt.Println("\n③ sendfile 是 Linux 专有的 syscall 形态（本机不是 Linux，跳过实测）")
	fmt.Println("   macOS 也有 sendfile，但签名不同，Go 的 syscall 包没有暴露；")
	fmt.Println("   本机的 io.Copy 会走用户态拷贝，所以②里的两条路径差异不明显。")
	fmt.Println("   到 Linux 上跑，sendfile 那条路上没有任何一次 CPU 拷贝。")
}
