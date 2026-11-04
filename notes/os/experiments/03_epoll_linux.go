//go:build linux

// # epoll 实验（笔记 03，仅 Linux）
//
// 对应笔记：notes/os/03-IO模型与epoll.md
//
// 运行：go run ./experiments/ epoll
package main

import (
	"fmt"
	"syscall"
)

// ExpEpoll 对比水平触发（LT）与边缘触发（ET）的通知次数。
func ExpEpoll() {
	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		fmt.Println("epoll_create1 失败：", err)
		return
	}
	defer syscall.Close(epfd)

	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_NONBLOCK); err != nil {
		fmt.Println("pipe2 失败：", err)
		return
	}
	rfd, wfd := fds[0], fds[1]
	defer syscall.Close(rfd)
	defer syscall.Close(wfd)

	// ① 水平触发：有数据就一直通知，即使我们不读
	epollCtl(epfd, syscall.EPOLL_CTL_ADD, rfd, syscall.EPOLLIN)
	if _, err := syscall.Write(wfd, []byte("x")); err != nil {
		fmt.Println("写入管道失败：", err)
		return
	}
	fmt.Println("① 水平触发（LT，默认）：写入 1 字节后不读，连续 epoll_wait 3 次")
	for i := 1; i <= 3; i++ {
		fmt.Printf("   第 %d 次 epoll_wait → 就绪事件数 %d\n", i, epollWaitCount(epfd))
	}

	// ② 边缘触发：只在状态变化时通知一次
	epollCtl(epfd, syscall.EPOLL_CTL_MOD, rfd, syscall.EPOLLIN|syscall.EPOLLET)
	drainPipe(rfd)
	fmt.Println("\n② 边缘触发（ET）：")

	if _, err := syscall.Write(wfd, []byte("x")); err != nil {
		fmt.Println("写入管道失败：", err)
		return
	}
	fmt.Printf("   写入 1 字节后 epoll_wait → 就绪事件数 %d（产生边沿，通知一次）\n", epollWaitCount(epfd))

	if _, err := syscall.Write(wfd, []byte("y")); err != nil {
		fmt.Println("写入管道失败：", err)
		return
	}
	fmt.Printf("   不读、再写 1 字节后 epoll_wait → 就绪事件数 %d（没有新边沿，不通知）\n", epollWaitCount(epfd))

	drainPipe(rfd)
	if _, err := syscall.Write(wfd, []byte("z")); err != nil {
		fmt.Println("写入管道失败：", err)
		return
	}
	fmt.Printf("   读空之后再写 1 字节 → 就绪事件数 %d（新的边沿，再次通知）\n", epollWaitCount(epfd))

	fmt.Println()
	fmt.Println("判读：")
	fmt.Println("  LT 会反复提醒，所以「读一次没读完」也不会丢事件——这是它安全的原因，")
	fmt.Println("  代价是就绪 fd 多时会重复唤醒。")
	fmt.Println("  ET 只提醒一次，因此必须循环读到 EAGAIN；漏读的后果是连接静默卡死，")
	fmt.Println("  而且 epoll_wait 再也不会为它返回——这是 ET 最典型的线上 bug。")
}

// epollCtl 注册/修改/删除关注的事件。
func epollCtl(epfd, op, fd int, events uint32) {
	ev := &syscall.EpollEvent{Events: events, Fd: int32(fd)}
	if err := syscall.EpollCtl(epfd, op, fd, ev); err != nil {
		fmt.Println("epoll_ctl 失败：", err)
	}
}

// epollWaitCount 等一次就绪，返回就绪事件数（超时返回 0）。
func epollWaitCount(epfd int) int {
	events := make([]syscall.EpollEvent, 8)
	n, err := syscall.EpollWait(epfd, events, 200)
	if err != nil {
		if err == syscall.EINTR {
			return 0
		}
		fmt.Println("   epoll_wait 失败：", err)
		return 0
	}
	return n
}

// drainPipe 把管道读空（非阻塞 fd，读到 EAGAIN 为止）。
func drainPipe(fd int) {
	buf := make([]byte, 64)
	for {
		if _, err := syscall.Read(fd, buf); err != nil {
			return
		}
	}
}
