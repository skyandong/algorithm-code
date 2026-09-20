# 运行时调度器 GMP

> **核心认知：** Go 调度器把「任务（G）」「执行流（M）」「执行资格与资源（P）」三者解耦，G 挂起时 M 立刻换一个跑——goroutine 便宜就便宜在这儿。区别只有一处：**阻塞在 channel 上是 G 挂起、M 不停；阻塞在系统调用里是 M 陪绑、P 被移交给别的 M**。

本文基于 Go 1.26（本机 1.26.3）的实际行为编写。

---

## 目录

1. [G / M / P：职责与数量关系](#1-g--m--p职责与数量关系)
2. [调度循环：runnext、本地队列、work-stealing](#2-调度循环runnext本地队列work-stealing)
3. [系统调用：M 与 P 分离（hand off）](#3-系统调用m-与-p-分离hand-off)
4. [网络 IO：netpoller](#4-网络-ionetpoller)
5. [抢占：协作式 → 基于信号的异步抢占](#5-抢占协作式--基于信号的异步抢占)
6. [GOMAXPROCS：语义与容器的坑](#6-gomaxprocs语义与容器的坑)
7. [goroutine 为什么轻](#7-goroutine-为什么轻)

---


## 1. G / M / P：职责与数量关系

| 角色 | 是什么 | 数量 |
| --- | --- | --- |
| G（goroutine） | 一段并发任务：栈 + PC + 状态（runnable/running/waiting/dead） | 无上限，受内存约束，百万级可行 |
| M（machine） | OS 线程，真正绑核执行的执行流 | 按需创建，有上限（`debug.SetMaxThreads` 可调） |
| P（processor） | 执行 G 的"许可证" + 资源包：本地运行队列、mcache、defer 栈等 | `=` GOMAXPROCS，默认 `runtime.NumCPU()` |

核心关系：

```text
M 必须拿到 P 才能执行 Go 代码
P 的数量固定（GOMAXPROCS），M 的数量随阻塞情况浮动
G 数量远大于 P/M：靠"排队 + 快速切换"复用执行流
```

整体结构：

```text
                     ┌───────────────┐
                     │   全局队列     │ ←─ 本地队列满时转移一半进来
                     └───────┬───────┘
        ┌────────────────────┼────────────────────┐
   ┌────▼─────┐         ┌────▼─────┐         ┌────▼─────┐
   │ P0       │ steal   │ P1       │  steal  │ P2       │
   │ runnext  │◄───────►│ runnext  │◄───────►│ runnext  │
   │ 本地队列 │ (偷一半) │ 本地队列 │ (偷一半) │ 本地队列 │
   └────┬─────┘         └────┬─────┘         └────┬─────┘
        │ M0 持有             │ M1 持有            │ M2 持有
   ┌────▼─────┐         ┌────▼─────┐         ┌────▼─────┐
   │ M0       │         │ M1       │         │ M2       │
   │ OS 线程  │         │ OS 线程  │         │ OS 线程  │
   └──────────┘         └──────────┘         └──────────┘
   （另有不占 P 的系统线程：sysmon 监控线程、大量空闲/阻塞的 M）
```

WHY M 按需浮动：线程创建和切换是有成本的（µs 级 + MB 级栈）。多数时候 GOMAXPROCS 个 M 就够；只有大量 G 同时陷在阻塞系统调用里时才需要更多 M。M 有上限防失控——真实世界「thread exhaustion」崩溃通常来自大量阻塞的 cgo 调用或第三方库的 `LockOSThread` 泄漏，而不是业务 goroutine。

---

## 2. 调度循环：runnext、本地队列、work-stealing

M 持有 P 后循环找 G 执行（简化自 runtime `findRunnable`）：

```text
1. runnext 槽      —— 最近刚就绪的 G，最高优先级（缓存还热着）
2. 本地队列         —— P 私有，定长数组（满时挪一半去全局），无锁访问
3. 全局队列         —— 加锁，每隔若干轮调度查一次，免得它被本地队列饿死
4. netpoll         —— 非阻塞地问 netpoller 有无就绪的网络 G
5. work-stealing   —— 从其他 P 的本地队列偷一半（"偷一半"摊薄偷的成本）
6. 都没有           —— M 睡眠（阻塞在 netpoll 上或等待唤醒）
```

三个关键设计及 WHY：

- **runnext 槽**：新创建/刚被唤醒的 G 先放 runnext。典型受益者是「channel 一发一收」的握手场景——接收方刚被唤醒就立刻执行，数据还在 CPU cache 里。
- **本地队列 256 + 满了转移一半到全局**：本地队列保证无锁；塞满时把一半移到全局队列，既是溢出兜底也是负载均衡（全局队列人人可取）。
- **偷一半而不是偷一个**：偷本身有成本（两次 CAS + 可能的竞争重试），一次偷半个队列能让"偷一次顶很久"，减少 P 之间互相频繁偷。

由此推出一条常被记反的顺序结论：**连续两条 `go` 语句，后创建的那个先跑**。后创建的 G 覆盖 `pp.runnext`，先创建的被挤到本地队列尾；单 P 且双方都不主动让出时，`findRunnable` 先取 runnext，顺序是确定的、不是随机的。前提是没开 race detector：`-race` 下 runtime 会以 1/2 概率放弃 runnext 来打散这类顺序假设（`const randomizeScheduler = raceenabled`，`src/runtime/proc.go`），此时先后不再确定——所以依赖调度顺序的断言必须带 `-race` 跳过。

```go
// 实验：runnext 让"唤醒后立刻执行"成为可能
ch := make(chan int)
go func() { v := <-ch; fmt.Println(v) }() // 接收方 park 在 recvq
ch <- 42                                   // 发送方 goready 接收者 → 放 runnext → 大概率下一个就跑
```

---

## 3. 系统调用：M 与 P 分离（hand off）

G 陷入**阻塞**系统调用（读慢速文件、pipe、cgo 调用等）时，M 被内核卡住，无法再服务其他 G。此时调度器把 P 从 M 手里拿走：

```text
G1 在 M1 上调用 read(fd)（慢速阻塞 fd）
  │
  ├─ entersyscall：P 进入 _Psyscall 状态（暂不接新 G），M1 带 G1 陷入内核
  │
  ├─ sysmon（独立监控线程，不需要 P，自适应地周期性巡视）发现 P 卡在
  │  _Psyscall 已超过一个监测周期
  │     └─ hand off：P 解绑 M1，交给别的 M（唤醒空闲 M 或新建）
  │
  └─ read 返回（exitsyscall）：M1 优先取回原 P → 取不到就抢空闲 P
       └─ 都失败：G1 放入全局队列，M1 挂起
```

WHY 分离而不是绑死：如果 P 跟着 M 一起阻塞在内核里，队列里成百上千的 runnable G 就没人服务了。把"执行资格"（P）和"执行流"（M）解耦，M 卡住就换一个 M 顶上，G 的调度照常运转。

注意区分快慢系统调用：非阻塞 read 返回 EAGAIN、`getpid` 这类"瞬回"syscall 不会触发 hand off（进出 syscall 的开销远小于 hand off 本身），只有 sysmon 观察到 P 长时间不回来才接手。

---

## 4. 网络 IO：netpoller

Go 网络代码写起来是阻塞风格，但 goroutine 阻塞在 socket 上**不占任何线程**：

```text
G1: conn.Read(buf)        ← fd 暂时无数据
  │
  ├─ runtime 把 fd 注册进 netpoller（Linux: epoll / macOS: kqueue / Windows: IOCP）
  ├─ G1 park（挂起，挂进 fd 的等待链）
  └─ M1 不等：立刻取下一个 G 继续跑
       ...
网卡收到数据，fd 就绪
  └─ netpoll 拿到就绪 G 列表 → goready 放回运行队列 → 某个 M 很快执行它
```

这就是 Go「一个连接一个 goroutine」模型的底气：十万长连接的 HTTP 服务，活跃 M 往往只有几十个——绝大多数 G park 在 netpoller 上，等事件就绪才回运行队列。

netpoller 的两个接入点：

1. 调度循环里的非阻塞 `netpoll(0)`（findRunnable 顺序第 4 步）；
2. 没事可做的 M 直接阻塞在 `netpoll(block)` 上等事件唤醒——线程数跟随负载自然伸缩。

（channel 阻塞、`time.Sleep` 也是同样的 park 模式：G 挂起、M 走人。）

---

## 5. 抢占：协作式 → 基于信号的异步抢占

### 5.1 协作式抢占：函数调用点的检查

编译器在**每个函数的序言**插入栈检查（为栈增长 `morestack` 预留的检查点），runtime 复用这个检查点做抢占：把 G 的 `stackguard0` 设成 `stackPreempt`，G 下一次函数调用进入检查逻辑时发现需要抢占，让出 CPU。

死穴：**没有函数调用的循环是盲区**。

```go
// 纯 CPU 循环，循环体内零函数调用 —— 没有协作式抢占点，无法让出
for {
    sum += i
    i++
}
```

WHY 这是致命问题：GC 开始/结束需要短暂 STW（stop the world），而 STW 要求所有 G 到达安全点。一个纯 CPU 循环能把 STW 从亚毫秒拖到几十毫秒甚至秒级（GC 卡住 → 内存堆积 → 延迟雪崩），也会把同 P 上的其他 G 饿死。

### 5.2 基于信号的异步抢占

sysmon 发现某个 G 连续运行超过了**时间片**（`forcePreemptNS`），直接向它所在的 M 发信号（类 Unix 平台用 **SIGURG**）：

```text
sysmon: G 超时间片
  └─ preemptM: 向 M 发 SIGURG
       └─ 信号处理器：确认是抢占请求 → 改写被中断的执行上下文
            └─ M 恢复执行时跳转到 asyncPreempt → 强制进入调度器让出 CPU
```

WHY 选 SIGURG：几乎无人在意它——SIGSEGV/SIGABRT 留给故障，SIGUSR1/2 常被程序自己占用，SIGURG（TCP 带外数据）现代程序基本不用，冲突概率最低。

异步抢占后，纯 CPU 循环最多独占 P 一个时间片就会被切走，GC STW 恢复亚毫秒。**但注意**：协作式检查点没有删除（函数调用点仍是最高效的抢占路径）；少数场景依旧不可抢占——持有 runtime 内部锁、处于 runtime 非安全点、个别汇编路径，以及阻塞在系统调用里的 G（不需要信号，走 hand off）。

---

## 6. GOMAXPROCS：语义与容器的坑

语义：**同时执行 Go 代码的 M 上限 = P 数量**。默认 `runtime.NumCPU()`（逻辑核，含超线程）。

### 6.1 k8s CPU limit 的经典事故

CFS quota 按 100ms 周期结算：`cpu: "2"` = 每周期 200ms CPU 时间，超了就**整个容器硬暂停到下个周期**（throttle）。

| 场景 | GOMAXPROCS | 后果 |
| --- | --- | --- |
| 64 核节点 + limit 2 核（未显式设置时） | 64（按宿主机核数） | 64 个 P 抢 2 核配额 → 每周期提前耗尽 → throttle 硬暂停 → 尾延迟尖刺（p99 从 10ms 飙到几百 ms）；GC 并发标记进一步烧配额 |
| 同上（Linux，runtime 默认行为） | 自动 = 2 | 正常 |
| 同上 + `uber-go/automaxprocs` | init 时读 cgroup 设好 | 等效民间方案 |

runtime 默认行为（Linux）：runtime 读 cgroup v2 CPU 带宽限制，quota 非整数时**向上取整**，且定期重读（cgroup 限制运行时可变）。手动设置优先：`GOMAXPROCS` 环境变量或 `runtime.GOMAXPROCS()` 调用会关闭自动行为；`GODEBUG=containermaxprocs=0`/`updatemaxprocs=0` 可单独禁用。CPU requests 不参与计算。

### 6.2 实战认知

- GOMAXPROCS=1：并发但不并行，goroutine 依然交错执行；
- 限核/高密度环境适当调小有时**更快**（减少线程争抢与 GC 辅助的并行开销）；
- 调大超过配额核数几乎只有害处（throttling 是硬暂停，不是温和降速）。

---

## 7. goroutine 为什么轻

| 维度 | goroutine | OS 线程 |
| --- | --- | --- |
| 初始栈 | KB 级，按需增长 | MB 级（Linux 默认软限制 8MB），创建即保留地址空间 |
| 创建成本 | ns~µs 级，只是初始化几个结构体 | µs 级 + 内核对象 + 栈 |
| 切换成本 | 用户态换寄存器（`gogo`），百 ns 级，不进内核 | 内核上下文切换，µs 级（缺页、cache/TLB 污染另算） |
| 可共存数量 | 百万级常规操作 | 千级就要精细调优 |

WHY 能做到：栈按需增长（`morestack` 分配更大的新栈 → 拷贝内容 → 重定向指针的"连续栈"方案）+ 切换不进内核（调度数据全在用户态）。代价：goroutine 不能像线程那样被内核直接调度、`LockOSThread` 才能绑定。

## 实验

对应代码：[experiments/07_gmp_test.go](experiments/07_gmp_test.go)（断言式单测，无打印）

```bash
cd notes/golang
go test ./experiments/          # 全部实验（约 4 秒）
```

实验内容：
1. GOMAXPROCS / NumCPU / NumGoroutine 基本观察与修改
2. 批量启动 1000 个 goroutine，观察 NumGoroutine 增长与回收
3. 单 P 下 `runtime.Gosched()` 确定性让出 vs 抢占的偶然性
4. 纯 CPU 循环 + tick：验证信号异步抢占不饿死其他 G
5. channel 阻塞不增线程 vs 阻塞系统调用增加线程（hand off 的直接观测）
6. 单 P 下连续两条 `go`：后创建者占住 runnext 先运行（`-race` 下跳过）
