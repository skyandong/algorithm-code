# IO 模型与 epoll

> **核心认知：** 一次网络读有两个阶段——**等数据就绪**和**把数据从内核搬到用户态**。五种 IO 模型的差别只在"这两个阶段谁阻塞、阻塞几次"。epoll 解决的是第一个阶段：让你用一个线程等成千上万个连接；第二个阶段的拷贝它一点没省——那要靠零拷贝。

---

## 1. 五种 IO 模型

```
阻塞 IO      : [====== 等数据 ======][== 拷贝 ==]     全程阻塞
非阻塞 IO    : [轮询 等数据 -> EAGAIN][== 拷贝 ==]     等的时候不阻塞，但要反复问
IO 多路复用  : [select/epoll 等就绪][== 拷贝 ==]       一个线程等多路，拷贝仍阻塞
信号驱动 IO  : [等信号][== 拷贝 ==]                    就绪时内核发 SIGIO
异步 IO      : [========= 内核全包 =========]          连拷贝完成才通知（io_uring）
```

| 模型 | 等数据 | 拷贝数据 | 典型代表 |
|------|-------|---------|---------|
| 阻塞 | 阻塞 | 阻塞 | 默认 `read` |
| 非阻塞 | 轮询（EAGAIN） | 阻塞 | `O_NONBLOCK` |
| 多路复用 | 阻塞在 `epoll_wait` | 阻塞 | epoll / kqueue |
| 信号驱动 | 不阻塞，等信号 | 阻塞 | `SIGIO`（几乎没人用） |
| 异步 | 不阻塞 | 不阻塞 | io_uring / POSIX AIO |

**注意一个高频误区**：epoll 不是"异步 IO"，它是**就绪通知**机制——真正的 `read` 还是你自己调、还是阻塞的。只有 io_uring 那种"提交读请求、完成后回调"才算异步。

---

## 2. select → poll → epoll 的演进动机

| | select | poll | epoll |
|---|---|---|---|
| fd 上限 | **1024**（`FD_SETSIZE`） | 无硬上限 | 无硬上限 |
| 每次调用 | 把整个 fd 集合**拷进内核** | 同上（数组） | **不用传**，注册一次即可（`epoll_ctl`） |
| 内核找就绪 | 遍历全部 fd，O(n) | 同 O(n) | 回调维护就绪链表，O(就绪数) |
| 返回后 | 你还要遍历判断哪个就绪 | 同 | 直接拿到就绪列表 |
| 触发模式 | 仅水平 | 仅水平 | 水平 + 边缘 |

**核心差异是"谁在维护就绪状态"**：

- select/poll：每次调用都重新扫一遍"这些 fd 现在谁就绪"——连接数 1 万时，每次调用扫 1 万次。
- epoll：fd 注册时把关注的事件挂进内核结构，**数据到达时由回调把节点塞进就绪链表**，`epoll_wait` 只取链表。所以连接数多但活跃少时（典型 Web 服务），代价与"活跃数"成正比而不是"连接数"。

---

## 3. epoll 的内核结构

```
       eventpoll（每个 epoll fd 一个）
       ┌──────────────────────────────────────────┐
       │  rbr   : 红黑树，存所有注册的 epitem       │  ← epoll_ctl 增删改
       │  rdllist: 就绪链表，epoll_wait 直接取      │  ← 数据到达时回调加入
       │  wq    : 等待队列，epoll_wait 阻塞在这     │  ← 有就绪时唤醒
       └──────────────────────────────────────────┘
              │                    ▲
              │ 每个被监听的 fd 一个 epitem │
              ▼                    │
       ┌──────────────┐    ep_poll_callback（socket 数据到达时被调用）
       │ epitem(fd=7) │────────────┘
       └──────────────┘
```

复杂度：`epoll_ctl` O(log n)（红黑树插入）、`epoll_wait` O(就绪数)。

**为什么普通文件不能被 epoll 监听**：文件 IO 永远"就绪"（没有等待语义），epoll 会立刻返回并让你空转。文件 IO 只能靠线程池或 io_uring。

---

## 4. 水平触发 vs 边缘触发

| | LT（水平触发，默认） | ET（边缘触发） |
|---|---|---|
| 通知时机 | 只要还有数据未读完，**每次 epoll_wait 都会通知** | 状态**变化**时通知一次（空→有数据） |
| 编程要求 | 读一次也行，下次还会提醒 | **必须循环读到 EAGAIN**，否则再也不通知 |
| fd 模式 | 阻塞或非阻塞都行 | **必须非阻塞**（否则最后一次读会永久阻塞） |
| 风险 | 就绪 fd 多时反复唤醒（可接受） | 漏读 = 连接"卡死"，且很难查 |

**ET 的经典 bug**：只 `read` 一次（读了 1KB，缓冲里还有 9KB），以为下次还会通知——结果再也不来，连接挂住。所以 ET 必须配 `for { n, err := read(); if err == EAGAIN { break } }`。

**选型**：绝大多数服务用 LT 就够了（nginx 默认也是 LT）。ET 的优势是减少唤醒次数，在百万连接、事件密集场景才值得。

---

## 5. 惊群与多核扩展

**惊群**：多个进程/线程阻塞在同一个监听 fd 的 `epoll_wait` 上，一个新连接到达唤醒**全部**等待者，只有一个能 accept，其余白醒。

| 方案 | 做法 | 适用 |
|------|------|------|
| `EPOLLEXCLUSIVE`（4.5+） | 注册时加标志，内核只唤醒一个 | 多线程共享同一 listen fd |
| `SO_REUSEPORT` | 每个worker 各自 `bind` 同一端口，内核按 hash 分流 | nginx 多 worker 的标准做法 |
| accept mutex（应用层） | 抢锁再 accept | 老办法，已过时 |

`SO_REUSEPORT` 是现在的主流：每个 worker 独立 epoll 循环，无共享、无惊群，内核按四元组 hash 分配。

---

## 6. Go 的 netpoller

Go 不用你手写 epoll，runtime 替你做了三层封装：

```
goroutine 发起 conn.Read
      │  fd 非阻塞，read 返回 EAGAIN
      ▼
netpollblock：把 G 挂到 pollDesc 的等待队列，gopark 让出 P
      │
      ▼
某个 M 在 epoll_wait 上阻塞（空闲 M / sysmon 定期调用）
      │  数据到达，epoll 返回就绪
      ▼
netpoll → goready(G) 把 G 放回运行队列，等 P 来执行
```

关键点：

- **你的代码是同步写法**（`conn.Read` 一行），但底层是 epoll + 非阻塞 fd + G 的挂起唤醒——这就是"同步的写法、异步的性能"。
- Linux 用 epoll，macOS/BSD 用 kqueue，Windows 用 IOCP——Go 用 build tag 各写一份。
- **没有后台线程池**：不需要"每个请求一个线程"，几万连接只需要很少的 M。

---

## 7. io_uring 与 Go 的现状

`io_uring` 用两块**共享内存环**（提交队列 SQ / 完成队列 CQ）替代 syscall：

- 应用往 SQ 写"读哪个 fd、读到哪"，一次 `io_uring_enter` 提交一批；
- 内核完成后往 CQ 写结果，应用直接读共享内存——**连"数据拷贝"都可以异步**（配合注册缓冲）。

它同时解决了两件事：syscall 次数（批量化）和真正的异步语义。代价是内核版本要求（5.1+，成熟特性要 5.10+）、安全面变大（曾有多起提权漏洞）。

**Go 标准库目前没有 io_uring 后端**，netpoller 仍是 readiness 模型。想用要引第三方库（如 `iceber/go-uring`），或者等 runtime 支持。

---

## 8. 记忆锚点（面试 30 秒版）

> "IO 分两阶段：等数据、搬数据。epoll 只优化第一阶段——注册一次、就绪靠内核回调塞进就绪链表，wait 的代价跟活跃连接数走而不是总连接数，所以一个线程能扛几万连接；但 read 的拷贝还是阻塞的，异步得靠 io_uring。LT 是还有数据就一直通知，ET 只在状态变化时通知一次、必须读到 EAGAIN 且 fd 非阻塞。Go 的 netpoller 就是这个模型：fd 非阻塞、G 挂起、M 跑 epoll_wait、就绪了 goready。"

---

## 实验

`go run ./experiments/ epoll`（Linux 上跑真实 epoll，其它平台跑等价说明）：

- 用 `epoll_create1` + 两个 pipe，对比 **LT 与 ET** 的通知次数
- LT：不读数据，`epoll_wait` 每次都返回就绪
- ET：不读数据，只返回一次；读空之后再有数据才会再通知

对应代码：[experiments/03_epoll_linux.go](experiments/03_epoll_linux.go)（Linux）/ [experiments/03_epoll_other.go](experiments/03_epoll_other.go)（其它平台）
