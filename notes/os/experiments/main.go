// # 操作系统实验统一入口（notes/os）
//
// 全部实验本地自包含，无需 root。Linux 专属实验（epoll / sendfile / cgroup）
// 用 build tag 区分：在 macOS 上自动降级为等价说明，编译与运行都不会失败。
//
// 用法:
//
//	go run ./experiments/           # 运行全部
//	go run ./experiments/ threads   # 实验1：协程数 vs 内核线程数
//	go run ./experiments/ sched     # 实验2：并行加速比
//	go run ./experiments/ epoll     # 实验3：LT 与 ET（Linux 真实 epoll）
//	go run ./experiments/ zerocopy  # 实验4：页缓存与零拷贝路径
//	go run ./experiments/ vm        # 实验5：虚拟内存与缺页
//	go run ./experiments/ memory    # 实验6：Go 堆与内核内存账本
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
	{"threads", "实验1：1 万 goroutine 阻塞时，内核线程数几乎不动", ExpThreads},
	{"sched", "实验2：串行 vs 并行 CPU 密集任务的加速比", ExpSched},
	{"epoll", "实验3：LT 反复通知 vs ET 只通知一次", ExpEpoll},
	{"zerocopy", "实验4：页缓存命中与 sendfile 拷贝路径", ExpZeroCopy},
	{"vm", "实验5：mmap 缺页与「申请不等于占用」", ExpVM},
	{"memory", "实验6：HeapReleased 与 RSS 不是一回事", ExpMemory},
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
	fmt.Println("用法: go run ./experiments/ [threads|sched|epoll|zerocopy|vm|memory|all]")
	for _, c := range cases {
		fmt.Printf("  %-9s %s\n", c.name, c.desc)
	}
}
