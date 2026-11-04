//go:build !linux

// # epoll 实验（笔记 03，非 Linux 平台的等价说明）
//
// 对应笔记：notes/os/03-IO模型与epoll.md
package main

import "fmt"

// ExpEpoll 在非 Linux 平台只输出说明：epoll 是 Linux 专有接口，
// macOS 的等价物是 kqueue，语义上 EV_CLEAR 对应边缘触发。
func ExpEpoll() {
	fmt.Println("本机不是 Linux，无法直接调用 epoll；这里输出等价说明（编译与运行均不失败）。")
	fmt.Println()
	fmt.Println("在 Linux 上运行 `go run ./experiments/ epoll` 会看到：")
	fmt.Println("  LT（默认）：写入 1 字节后不读，连续 3 次 epoll_wait 全部返回就绪 —— 水平触发反复提醒")
	fmt.Println("  ET：只有第一次返回就绪；不读再写也不会通知；读空后再写才会出现新的边沿")
	fmt.Println()
	fmt.Println("对应的内核语义：")
	fmt.Println("  LT：只要 fd 上还有未处理事件，每次都放进就绪链表")
	fmt.Println("  ET：只有状态发生变化（无数据 → 有数据）才挂一次就绪")
	fmt.Println()
	fmt.Println("Go 的 netpoller 在 macOS 上走 kqueue：EVFILT_READ + EV_CLEAR 就是边缘触发语义，")
	fmt.Println("但这层差异由 runtime 屏蔽了，业务代码仍然按阻塞的样子写。")
}
