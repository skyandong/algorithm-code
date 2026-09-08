# Prometheus 架构与抓取：为什么是 pull

> **核心认知：** Prometheus 最反直觉也最被追问的设计是**它主动去拉你的服务，而不是你的服务推给它**。这不是历史包袱——pull 让"监控目标是否活着"这个问题从"数据有没有到"降级成了"HTTP 探一下就知道"，让服务发现天然可做、让本地 `curl /metrics` 就能排障、让应用侧不需要知道监控系统的地址。**代价是它抓不了短命任务，于是有了 Pushgateway 这个有明确边界的补丁**（不是通用的 push 通道）。想清楚"pull 换来了什么、在哪种场景换不来"，这一节就成了。

前置知识：[07-指标体系与数据模型](07-指标体系与数据模型.md)（series、四类指标、exposition 格式）。

---

## 目录

1. [pull 与 push：五个维度的对比](#1-pull-与-push五个维度的对比)
2. [组件全景](#2-组件全景)
3. [TSDB：内存里两个部分、磁盘上两类文件](#3-tsdb内存里两个部分磁盘上两类文件)
4. [抓取配置与三个时间参数](#4-抓取配置与三个时间参数)
5. [服务发现：目标从哪来](#5-服务发现目标从哪来)
6. [扩展：联邦、远程写与 Thanos](#6-扩展联邦远程写与-thanos)
7. [Pushgateway 的唯一正当用途](#7-pushgateway-的唯一正当用途)
8. [面试高频](#8-面试高频)

---

## 1. pull 与 push：五个维度的对比

| 维度 | pull（Prometheus） | push（StatsD / 商业 APM） |
|---|---|---|
| 目标存活判定 | **抓取失败本身就是判定**（`up=0`） | 无法区分"进程死了"和"数据没来" |
| 配置位置 | 集中在监控侧，改抓取目标不用动应用 | 应用侧要配监控后端地址 |
| 调试 | `curl host:port/metrics` 直读 | 只能去后端界面看，或抓包 |
| 数据完整性 | 抓取失败则**该点缺失**（`rate` 能容忍） | 客户端可缓冲重发 |
| 短命任务 | **抓不到**（抓之前就退出了） | 天然支持 |

**WHY pull 赢了**：微服务场景下目标数量庞大且频繁变化（扩缩容、发版），**"监控谁"这件事必须能动态发现**——pull 模式下 Prometheus 问一次服务发现（K8s API / Consul）就知道该抓谁，push 模式下你得让成百上千个应用各自维护一份"我要发给谁"的配置。而 `up` 指标这个免费的存活探针，本身就是 pull 模式最值钱的副产品：**`up == 0` 是最基础的告警，push 模式下你要另建探活系统才能做到**。

**pull 的真实代价**：①被抓方必须暴露 HTTP 端点，NAT/防火墙后的目标抓不到（需要 agent 或代理）；②采样点缺失（网络抖动丢一次抓取，`rate` 会用邻近点外推，精度下降但不报错）；③短命任务抓不到。前两个是工程问题，**第三个才是模式缺陷，所以 Pushgateway 只针对它**。

---

## 2. 组件全景

```text
                          ┌──────────────┐
   服务发现 (K8s/Consul) ─→│              │
                          │  Prometheus  │─┬─→ 告警: 推给 Alertmanager
   抓取目标 /metrics ─────→│   Server     │ │
                          │              │ └─→ 查询: PromQL HTTP API
   Pushgateway ──────────→│  (本地 TSDB)  │        ↑
                          └──────────────┘        │
                                 ↑                │
                          Recording/Alerting Rules│
                                                  │
                          Grafana ────────────────┘
```

- **Prometheus Server**：抓取 + 存 TSDB + 执行规则 + 提供 PromQL。单体、无集群依赖——**它自己就是一个完整系统**，这是它能单机跑在笔记本上的原因。
- **Alertmanager**：独立进程，只做告警的**去重/分组/抑制/静默/路由**。Prometheus 只负责"算出这条规则触发了"，其余全交给它（见 10 篇）。
- **Exporter**：把第三方系统（MySQL、Redis、nginx、机器）的内部状态翻译成 `/metrics` 文本。被监控者不懂 Prometheus 时的适配器（见 13 篇）。
- **Grafana**：只做展示，通过 HTTP 查 PromQL。它**不存数据**，换掉它 Prometheus 毫无损失。

**WHY Alertmanager 要独立**：告警的"路由到人/群/值班表、抑制、静默"是**运营逻辑**，变更频率远高于监控配置，且需要独立于监控系统的 HA。把运营逻辑塞进采集系统会互相拖累——这与 04 篇"网关把横切关注点从业务剥离"是同一个设计动作。

---

## 3. TSDB：内存里两个部分、磁盘上两类文件

Prometheus 为自己的负载做了定制存储（不是通用 TSDB，也不是 LSM 树通用引擎）。理解三层就够：

```text
写入路径:
  抓取样本 → 写入 head block（内存, 最近 ~2h）+ 追写 WAL（磁盘, 防崩）
                    ↓ 每 2 小时
            head block 落盘成不可变 block（磁盘, 自带倒排索引）
                    ↓ 后台 compaction
            多个小 block 合并成大 block（并降采样）

查询路径:
  PromQL → 内存 head block + 磁盘上时间范围内所有 block → 合并去重 → 返回
```

**三个关键设计**：

**① WAL（write-ahead log）**：进程崩了，内存里的 head block 全丢——WAL 就是重放日志，重启后重放恢复最近数据。**这是"监控不能因为自己崩了就丢数据"的底线**。

**② 不可变 block + 倒排索引**：落盘后的 block 永不再改，所以能顺序写、能高效压缩（时间戳 delta-of-delta，一个点压到 ~1.4 字节）。每个 block 自带"label → series → 样本位置"的倒排索引，**查询时先在索引里把 series 筛出来，再去读那一小段样本**——所以 `sum by (route)` 这种聚合才不会全表扫。

**③ 保留期（retention）**：默认 15 天，本地磁盘就是上限。**这不是缺陷而是定位**：Prometheus 的本地存储定位是"最近两周的高精度热数据"，长期存储交给远程写（见下）。

**为什么时间戳用抓取时刻而不是上报时刻**：同一轮抓取里所有目标的样本共享同一时间戳，才能构成一致的全局快照；如果允许应用自带时间戳，时钟偏移会让聚合结果不可解释。这也是 Pushgateway 被滥用的危害之一（它的样本时间戳是"推的时刻"，会和真实业务时刻错位）。

---

## 4. 抓取配置与三个时间参数

```yaml
scrape_configs:
  - job_name: pay-service
    scrape_interval: 15s      # 多久抓一次（默认继承 global）
    scrape_timeout: 10s       # 单次抓取超时
    metrics_path: /metrics
    kubernetes_sd_configs: [...]   # 见下节
```

**三个参数的关系与取值纪律**：

- `scrape_interval`：**必须 ≤ scrape_timeout 之外还要留余量**（timeout 通常设为 interval 的 1/2~2/3，否则慢抓取会堆积）。15s 是通用起点，核心服务可到 5s——**但每缩短一倍，样本量和成本翻倍**。
- `scrape_timeout`：超时的抓取记为失败（`up=0`，样本缺失）。
- `evaluation_interval`：规则求值间隔（告警规则多久算一次），**与 scrape_interval 解耦但通常设为同值**——比抓取快没意义（数据没更新），比抓取慢会延迟告警。

**`rate()` 的窗口必须 ≥ 4 倍 scrape_interval**（这是硬经验）：窗口内至少要有 4 个样本点，少一个都会让 rate 的估算失真或直接算不出（见 09 篇）。所以 15s 抓取配 `[1m]`，30s 抓取配 `[2m]`，**别用 `[10s]` 配 15s 抓取**——这是新手最常见的错误。

---

## 5. 服务发现：目标从哪来

静态 `static_configs` 只适合固定几台机器。生产靠服务发现：

| 机制 | 适用 | 说明 |
|---|---|---|
| `kubernetes_sd_configs` | K8s | 按 Pod/Service/Endpoint 自动发现，配合 `relabel_configs` 过滤 |
| `consul_sd_configs` | 非 K8s 但用了 Consul | 与 01 篇注册中心复用同一份名单 |
| `file_sd_configs` | 任何场景的兜底 | 读一个 JSON/YAML 文件，外部系统写它——**万能逃生舱** |
| `ec2_sd_configs` / 云厂商 | 云主机 | |

**relabel 是服务发现的伴侣**：发现出来的目标带一堆元数据（`__meta_kubernetes_pod_name` 等，`__` 前缀的标签抓取后会被丢弃），`relabel_configs` 决定"要不要抓、job 叫什么、label 怎么改"。**`metric_relabel_configs` 则在指标入库前改标签——丢弃高基数标签就在这里做**（07 篇第 4 节 的治理手段之一）。

---

## 6. 扩展：联邦、远程写与 Thanos

单机 Prometheus 的三个天花板：**存储容量**（单机磁盘）、**保留期**（15 天）、**全局视图**（多台 Prometheus 各自为政）。

| 方案 | 做法 | 解决 |
|---|---|---|
| **联邦（federation）** | 上层 Prometheus 抓下层 Prometheus 的 `/federate` 端点（只取聚合后的结果） | 全局视图；**必须抓聚合结果，不能抓原始 series**（否则数据量不变） |
| **远程写（remote_write）** | Prometheus 把样本实时转发给外部存储（Thanos/Mimir/Cortex/VictoriaMetrics） | 长期存储 + 无限保留 |
| **远程读（remote_read）** | 查询时回查外部存储 | 查历史数据 |
| **Thanos** | Sidecar + 对象存储 + Query 组件（联邦查询） | 上述三件事的一站式方案，加**降采样** |

**WHY 联邦必须聚合**：联邦的本意是"用精度换规模"——下层保留原始明细，上层只要 `sum by (job)` 这种聚合后的少量 series。如果上层原样抓下层的全部 series，等于把数据量搬了个家，**什么都没解决**。

**选型直觉**：中小规模（series 百万级以内、单机能扛）→ 单机 Prometheus + 远程写备份；大规模 + 多集群 → Thanos 或 VictoriaMetrics 集群版。

---

## 7. Pushgateway 的唯一正当用途

**它只该用于"短命的批处理任务"**：cron job / 离线任务跑完就退出，Prometheus 永远抓不到它，于是任务在退出前把结果推到 Pushgateway 上"寄存"，Prometheus 再去抓 Pushgateway。

**四个必须知道的坑**（面试爱问"能不能用 Pushgateway 做通用 push 通道"，答案是不能）：

1. **数据永不过期**：推上去的指标会一直在，除非任务自己删（用 `DELETE` 或 `pushgateway` 的 `job` 维度清理）。死任务留下的幽灵指标会一直触发告警。
2. **单点**：Pushgateway 挂了，所有批任务的监控就断了。
3. **没有 `up` 语义**：Prometheus 看到的是 Pushgateway 活着，不是任务活着——**你失去了 pull 模式最值钱的存活判定**。
4. **时间戳错位**：样本时间戳是"推的时刻"，不是"任务跑的时刻"，聚合结果会与真实业务时段错位。

所以：**长期运行的服务一律直接暴露 `/metrics` 让 Prometheus 拉**，Pushgateway 是批任务的特例通道，不是架构选项。

---

## 8. 面试高频

**Q1：Prometheus 为什么用 pull 而不是 push？**
核心是三件事：①抓取失败即存活判定（`up=0` 是最基础的告警，push 模式要另建探活）；②目标发现集中在监控侧（扩缩容时应用不需要知道监控在哪）；③`curl /metrics` 就能排障。代价是抓不到短命任务，所以 Pushgateway 只作为批任务的特例补丁存在。

**Q2：Prometheus 存储是怎么组织的？**
写入先进内存 head block 并追写 WAL（防崩）；每 2 小时 head block 落盘成不可变 block（自带倒排索引，时间戳 delta-of-delta 压缩到 ~1.4 字节/点）；后台 compaction 合并小 block。查询时合并内存 head 与磁盘上范围内所有 block。保留期默认 15 天，长期存储靠 remote_write。

**Q3：scrape_interval、scrape_timeout、evaluation_interval 的关系？**
timeout 要小于 interval（通常 1/2~2/3）避免抓取堆积；evaluation_interval 与 scrape_interval 通常取同值（更快没数据，更慢延迟告警）。纪律：`rate()` 的窗口 ≥ 4 倍 scrape_interval。

**Q4：高基数除了改代码还能怎么治？**
抓取侧用 `metric_relabel_configs` 丢弃标签（入库前生效）；用 recording rule 预聚合降低查询基数；告警监控 `prometheus_tsdb_head_series` 看趋势。

**Q5：单机 Prometheus 的瓶颈怎么破？**
联邦（上层抓下层的聚合结果，不是原始 series）、remote_write 到 Thanos/Mimir/VictoriaMetrics 做长期存储、remote_read 查历史。Thanos 是这三件事的一站式方案并带降采样。

**Q6：Pushgateway 能不能当通用 push 通道？**
不能。它只适合短命批任务。四个坑：数据不过期（幽灵指标持续告警）、单点故障、丢失 `up` 存活语义、时间戳是推的时刻而非业务时刻会与真实时段错位。长期服务一律直接暴露 `/metrics`。

**Q7：为什么 Alertmanager 要独立部署？**
告警的路由/抑制/静默是运营逻辑，变更频率远高于采集配置，且需要独立 HA。把运营逻辑塞进采集系统会互相拖累——把横切关注点剥离出去，与网关收敛治理逻辑同构。

---

本篇对应实验：experiments/13_exporter.go（自定义 exporter：模拟把第三方系统的内部状态翻译成 `/metrics`，并起一个 HTTP 端点供本机 Prometheus 抓取）、[monitoring/](monitoring/)（真实 Prometheus + Grafana 的 docker 配置与抓取验证）
