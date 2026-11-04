# CPU 调度与 cgroup

> **核心认知：** 调度器只做一件事——在**可运行的线程多于 CPU 时决定谁上**。它优化的是"公平 + 低延迟"，不是"最大化吞吐"。而容器时代真正决定你拿到多少 CPU 的不是 nice，是 **cgroup 的 quota**：超了就被节流（throttled），表现为延迟毛刺而不是 CPU 打满。

---

## 1. 调度的基本对象与目标

调度的单位是**线程**（Linux 的 task_struct），不是进程。要平衡两个矛盾目标：

| 目标 | 手段 | 代价 |
|------|------|------|
| 吞吐 | 少切换、长时片、缓存友好 | 交互延迟变差 |
| 延迟 | 抢占、短时片、优先唤醒 | 切换开销、cache 失效 |

Linux 默认是**抢占式**调度：更高优先级的任务可运行就抢占当前任务（CFS 里靠 tick 与唤醒时的 preempt check）。

---

## 2. CFS：按权重分配时间片

CFS（完全公平调度器）的思路：**记录每个任务"已经运行了多久"，永远挑"最欠账"的那个跑**。

```
vruntime += 实际运行时间 × (NICE_0_WEIGHT / 任务权重)
```

- 权重来自 nice 值：nice 0 = 1024，**每降 1 级约 ×1.25**（nice -20 ≈ 88761，nice 19 = 15）。所以 nice 差 10 级，CPU 份额差 ~10 倍。
- 选取：红黑树按 vruntime 排序，取最左节点。
- 时片：`sched_latency`（默认 6ms）按权重分摊到各任务，但有下限 `min_granularity`（0.75ms）——任务太多时不是无限切分，而是拉长周期。
- 新任务/wakeup 后的 vruntime 会被"补齐"到 `min_vruntime` 附近，防止刚唤醒的任务因为欠账少而长时间霸占 CPU。

Linux 6.6 起默认调度器换成了 **EEVDF**（用 lag 与 eligibility 选任务），但"按权重公平分配"的思想没变，nice 的语义也没变。

**实时任务**（`SCHED_FIFO` / `SCHED_RR`）优先级高于所有 CFS 任务——一个死循环的实时线程能把整台机器饿死，所以线上很少给业务进程配实时策略。

---

## 3. 多核：每核一个队列 + 负载均衡

```
CPU0 [rq: G1 G3 G7]   CPU1 [rq: G2]   CPU2 [rq: G4 G5 G6 G8]
        ↑ 各自无锁调度，定期被 load_balance 拉平
```

- 每个 CPU 有自己的 `rq`（运行队列），避免全局锁（这正是 Go 从 G-M 模型演进到 G-M-P 的同一动机）。
- **负载均衡**：周期性 / 唤醒时把任务从忙核迁到闲核，迁移本身有缓存失效成本，所以不能过频。
- **CPU 亲和性**：`taskset -c 0-3` 或 `sched_setaffinity`，把进程钉在固定核上——好处是缓存局部性强、延迟可预测，坏处是负载不均。
- **NUMA**：跨 socket 访问内存延迟高一个量级，所以数据库类服务通常配亲和 + 绑核。

---

## 4. 上下文切换：两种要分清

```bash
vmstat 1                    # cs 列：每秒上下文切换数（系统级）
pidstat -w -p <pid> 1       # cswch/s（自愿）与 nvcswch/s（非自愿）
```

| 指标 | 含义 | 高了说明什么 |
|------|------|-------------|
| `cswch/s` 自愿切换 | 任务主动让出（等 IO、等锁、sleep） | 阻塞等待多；偏高未必是坏事 |
| `nvcswch/s` 非自愿切换 | 被抢占 / 时片用完 | **CPU 争抢**，通常是并发度过高或 CPU 不足 |
| `cs` 总量 | 系统级切换次数 | 需要上下对照，单看无意义 |

**判读要点**：`nvcswch` 高 + CPU 使用率接近 100% = 真的缺 CPU；`cswch` 高 + CPU 不满 = 大量等待（锁竞争、IO 阻塞），该去查锁或下游。

Go 里能直接看调度延迟：`runtime/metrics` 的 `/sched/latencies:seconds`（goroutine 从可运行到真正被执行的等待时间），这是**比 CPU 使用率更能反映"卡不卡"的指标**。

---

## 5. cgroup v2：容器真正的 CPU 上限

容器不是虚拟机，它靠 cgroup 限制资源。CPU 相关三个文件：

```bash
cat /sys/fs/cgroup/cpu.max        # 形如 "200000 100000" = 每 100ms 周期最多用 200ms = 2 核
cat /sys/fs/cgroup/cpu.weight     # 相对权重 1~10000，默认 100（只在争抢时生效）
cat /sys/fs/cgroup/cpu.stat       # nr_periods / nr_throttled / throttled_usec
```

**关键区别**：

- `cpu.max` 是**硬上限**（带宽控制）：用超了就被**强制节流**，直到下个周期。
- `cpu.weight` 是**相对份额**：只有同一层级内有人抢 CPU 时才起作用，空闲时可以超出。

**这就是容器里最常见的"CPU 毛刺"来源**：quota 用尽 → 整个 cgroup 被 throttle 到下一个 100ms 周期 → 这段时间内所有线程停摆。监控上 CPU 使用率只有 50%~70%（因为一半时间被节流了），但 P99 延迟很难看。

判断依据：

```bash
grep throttled /sys/fs/cgroup/cpu.stat     # nr_throttled 持续增长 = 被限流了
```

处置方向：调大 `cpu.max`、减少 GOMAXPROCS 让 Go 别开太多线程去抢那点配额、或把 CPU 密集任务挪出该容器。

---

## 6. Go 与 CPU 上限的关系

- **GOMAXPROCS 决定 P 的数量**，也就是同时跑 Go 代码的线程上限。默认 = 核数。
- Go 1.25 起 **GOMAXPROCS 自动感知 cgroup 的 CPU limit**（之前必须手动设或引 uber-go/automaxprocs），所以 2 核配额的容器里不会再开 96 个 P。
- GOMAXPROCS 设**过大**的代价：多余的 P/M 抢不到 CPU → 高 `nvcswch` + GC 的 mark worker 与业务抢核 → 延迟变差。
- GOMAXPROCS 设**过小**的代价：CPU 闲置（尤其是有阻塞 IO 的服务）。

---

## 7. 记忆锚点（面试 30 秒版）

> "CFS 按权重公平分配：vruntime 累积实际运行时间除以权重，红黑树取最欠账的跑，nice 每级约 1.25 倍。每核一个运行队列，靠负载均衡拉平。切换分自愿和非自愿，nvcswch 高说明缺 CPU，cswch 高说明在等锁或 IO。容器里真正管 CPU 的是 cgroup 的 cpu.max 硬配额——超了被 throttle 到下一个周期，表现是 P99 毛刺而不是 CPU 打满。"

---

## 实验

`go run ./experiments/ sched` 测并行度与调度开销：

- 一个 goroutine 做 CPU 密集计算 vs NumCPU 个 goroutine 做同样总量的计算，比较墙钟时间
- 输出并行加速比，以及进程的 user/sys CPU 时间（`getrusage`）
- 加速比明显小于核数，说明有争抢或串行段

对应代码：[experiments/02_sched.go](experiments/02_sched.go)
