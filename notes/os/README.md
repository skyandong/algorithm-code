# 操作系统学习笔记

> 目标：把「进程线程 / 调度 / IO 模型 / 内存」从概念变成能观测、能归因的知识。6 篇按「谁在跑 → 谁上 CPU → 怎么等 IO → 内存怎么用」推进，每篇配可运行实验（Linux 专属实验在其它平台自动降级为等价说明）。

## 目录

1. [进程与线程](01-进程与线程.md) — task_struct、clone 共享粒度、切换成本、G/M/P 三层
2. [CPU 调度与 cgroup](02-CPU调度与cgroup.md) — CFS 权重、上下文切换、cgroup cpu.max 节流
3. [IO 模型与 epoll](03-IO模型与epoll.md) — 五种模型、epoll 内核结构、LT/ET、Go netpoller
4. [零拷贝与页缓存](04-零拷贝与页缓存.md) — 4 次拷贝、sendfile/mmap/splice、Direct IO
5. [虚拟内存与缺页](05-虚拟内存与缺页.md) — 页表与 TLB、minor/major fault、COW、RSS/PSS
6. [内存回收与 OOM](06-内存回收与OOM.md) — 水位与直接回收、cgroup 账本、OOM 选谁、Go 内存谜题

## 实验

全部本地自包含，无需 root。Linux 专属实验在 macOS 上自动降级为等价说明（build tag 区分）：

```bash
go run ./experiments/           # 全部
go run ./experiments/ threads   # 1. 协程数涨、内核线程数不涨
go run ./experiments/ sched     # 2. 并行加速比与调度开销
go run ./experiments/ epoll     # 3. LT 与 ET 的通知次数差异（Linux 真实 epoll）
go run ./experiments/ zerocopy  # 4. 页缓存命中与 sendfile 路径
go run ./experiments/ vm        # 5. mmap 缺页统计
go run ./experiments/ memory    # 6. Go 堆与内核内存账本
```

| 实验 | 眼见为实什么 |
|------|-------------|
| threads | 1 万 goroutine 阻塞时 `NumGoroutine` 涨到 1 万，`threadcreate` 几乎不动 |
| sched | 并行加速比 < 核数时，说明有争抢或串行段 |
| epoll | LT 反复通知、ET 只通知一次（不读干净就再也不通知） |
| zerocopy | 第二次读同一文件快一个量级；`io.Copy` 走 sendfile 而 `bufio` 包装后退化 |
| vm | 申请内存不写不占物理页；逐页读 mmap 文件产生 major fault |
| memory | `HeapReleased` 涨了 RSS 未必立刻降；cgroup 的 anon/file 分布 |

## 重点回顾（自测）

- [ ] 进程与线程共享什么、独有什么；`clone` 的标志位各管什么
- [ ] 线程切换为什么比进程切换便宜；goroutine 切换为什么不陷内核
- [ ] Go 的 M 是内核线程；阻塞系统调用为什么会新建 M
- [ ] CFS 的 vruntime 与 nice 权重关系；nvcswch 高说明什么
- [ ] cgroup `cpu.max` 与 `cpu.weight` 的区别；throttled 为什么造成 P99 毛刺
- [ ] 五种 IO 模型的两个阶段；epoll 为什么是 O(就绪数) 而不是 O(连接数)
- [ ] LT 与 ET 的编程约束；ET 漏读的后果
- [ ] read+write 的 4 次拷贝；sendfile 省掉了哪两次；mmap 为什么不一定更快
- [ ] minor 与 major fault 的区别；COW 的触发时机
- [ ] RSS 与 PSS 的区别；为什么 RSS 会虚高
- [ ] 水位线与直接回收；`si/so` 非零意味着什么
- [ ] OOM 选谁；怎么确认进程是被 OOM 杀的
- [ ] `GOGC` 与 `GOMEMLIMIT` 的分工；为什么 GOMEMLIMIT 要低于容器 limit

## 与其它模块的关系

- Go 调度器 GMP、GC 与内存管理的语言侧视角见 [golang](../golang/README.md)
- 网络 IO 与 epoll 在服务端的落地见 [network](../network/README.md)
- 容器 CPU/内存指标的监控与告警见 [microservice](../microservice/README.md)
