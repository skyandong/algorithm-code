# Exporter 速查：让不懂 Prometheus 的系统开口说话

> **核心认知：** Exporter 是**适配器**——MySQL、Redis、nginx、操作系统都不会自己输出 Prometheus 文本格式，Exporter 替它们做翻译：**去问一次内部状态（`SHOW STATUS` / `/stub_status` / `/proc`），把结果变成 `/metrics` 文本**。理解这个定位后有两个推论：①Exporter 本身**不存数据**（它只是查询时的翻译层，每次抓取现问现答）；②**一个 Exporter 可以代理多个实例**（mysqld_exporter 连 10 个 MySQL，用 `instance` 参数区分）——这与"每个进程自己埋点"是不同的部署形态，别混淆。

前置知识：[07](07-指标体系与数据模型.md)、[08](08-Prometheus架构与抓取.md)。联动：`notes/nginx`、`notes/redis`、`notes/mysql`、`notes/akafka`。

---

## 目录

1. [两种形态：直埋 vs Exporter](#1-两种形态直埋-vs-exporter)
2. [常用 Exporter 速查表](#2-常用-exporter-速查表)
3. [每个中间件该看什么指标](#3-每个中间件该看什么指标)
4. [nginx 的监控：stub_status 与日志](#4-nginx-的监控stub_status-与日志)
5. [手写一个 Exporter：Collector 模式](#5-手写一个-exportercollector-模式)
6. [blackbox：从外部探测](#6-blackbox从外部探测)
7. [面试高频](#7-面试高频)

---

## 1. 两种形态：直埋 vs Exporter

| | 直埋（instrumentation） | Exporter |
|---|---|---|
| 适用 | **自己的代码** | **第三方系统 / 闭源组件** |
| 位置 | 业务进程内 | 独立进程（sidecar 或集中部署） |
| 数据 | 进程内实时内存 | 抓取时现查目标系统 |
| 举例 | 你的 Go 服务（12 篇） | mysqld_exporter、redis_exporter |

**为什么 Exporter 要独立进程而不是内置到被监控系统里**：MySQL 不会为了你的监控去改代码。**Exporter 是"不改动被监控方"前提下的唯一解**——这也是它与 sidecar 模式（06 篇）同源的地方：**把横切能力做成邻居进程**。

**集中式 vs 一对一**：mysqld_exporter 一个进程可以监控多个 MySQL（`?target=` 参数指定），适合中间件数量多但监控项固定的场景；node_exporter 则必须每个机器一个（它读的是本机 `/proc`）——**取决于数据来源是"本机"还是"可远程查询"**。

---

## 2. 常用 Exporter 速查表

| Exporter | 监控对象 | 部署形态 | 关键指标 |
|---|---|---|---|
| **node_exporter** | Linux 主机 | 每机一个（DaemonSet） | CPU、内存、磁盘 IO、网络、文件描述符 |
| **cAdvisor** | 容器 | 每机一个 | 容器 CPU/内存/网络（K8s 里 kubelet 已内置） |
| **mysqld_exporter** | MySQL | 集中式（可多实例） | 连接数、QPS、慢查询、主从延迟、InnoDB 缓冲池命中率 |
| **redis_exporter** | Redis | 集中式 | 内存、命中率、连接数、key 数、主从复制偏移 |
| **kafka_exporter** | Kafka | 集中式 | **消费 lag**、分区数、ISR 收缩、broker 存活 |
| **nginx-prometheus-exporter** | nginx | 每机一个 | 活跃连接、请求速率、各状态响应数 |
| **blackbox_exporter** | **任意端点**（探测） | 集中式 | HTTP 状态码、响应时间、证书到期、DNS 解析 |
| **process-exporter** | 任意进程 | 每机一个 | 进程 CPU/内存/句柄数/线程数 |

**选型原则**：先看有没有官方/社区成熟的，**不要自己写**——Exporter 的坑（连接池管理、超时、目标不可用时返回什么）比看起来多。自己写只在"内部自研中间件"时才合理。

---

## 3. 每个中间件该看什么指标

**MySQL**（联动 `notes/mysql`）：

```text
饱和度（先行）: mysql_global_status_threads_connected / max_connections   ← 连接池打满是最常见故障
错误:          mysql_global_status_aborted_connects（被拒连接）
慢查询:        mysql_global_status_slow_queries 的 rate
主从延迟:      mysql_slave_status_seconds_behind_master   ← 读写分离架构的命门
死锁:          innodb_deadlocks
缓冲池命中率:  innodb_buffer_pool_reads / read_requests（低 = 磁盘 IO，查询变慢的根因）
```

**Redis**（联动 `notes/redis/04`）：

```text
饱和度:   redis_memory_used_bytes / maxmemory     ← 到 100% 就开始按策略淘汰 key
命中率:   keyspace_hits / (hits + misses)         ← 掉到 90% 以下要查（缓存穿透/大 key 淘汰）
连接数:   redis_connected_clients
延迟:     redis_commands_duration_seconds
主从:     master_link_up（0 = 复制断了）、master_offset 差值 = 复制积压
```

**Kafka**（联动 `notes/akafka/09`）：

```text
消费 lag:  kafka_consumergroup_lag             ← 唯一最重要的指标, 积压就是故障前兆
ISR:       kafka_topic_partition_in_sync_replica_count < 副本数  ← 副本掉队, 再掉一个就可能丢数据
生产:      kafka_topic_partition_current_offset 的 rate
存活:      kafka_brokers
```

**消费 lag 为什么是头号指标**：lag 上涨 = 消费跟不上生产，**在用户感知到之前就有几小时的预警窗口**。这是 05 篇"饱和度是先行指标"在 Kafka 上的具体形态。lag 告警的正确做法是**按消费速率预测多久能追上**（`predict_linear`），而不是拍一个固定条数。

---

## 4. nginx 的监控：stub_status 与日志

联动 `notes/nginx`。两个数据源各有分工：

**① stub_status（实时连接状态，机器视角）**：

```nginx
location /nginx_status {
    stub_status;
    allow 127.0.0.1;    # 只给本机 exporter 访问
    deny all;
}
```

输出七个数字：`Active connections` / `accepts` / `handled` / `requests` / `Reading` / `Writing` / `Waiting`。

**关键推导**：`accepts - handled` = **被拒绝的连接数**（worker_connections 打满或 accept 队列溢出）。`Writing` 持续高 = 上游（upstream）响应慢，连接卡在回写——**这是 nginx 层能给出的、关于后端变慢的最早信号**（`notes/nginx/05` 的排查项在这里有了监控抓手）。

**② access log（请求明细，业务视角）**：stub_status 只有连接数，**拿不到状态码分布、延迟分布、按路由的 QPS**。要这些得解析 access log（nginx-prometheus-exporter 的 `-nginx.scrape-uri` 配合 `log_format` 输出 JSON，或用 mtail / Vector 做日志转指标）。

**取舍**：stub_status 零成本、信息少；access log 解析成本高（每行一次正则）但信息全、能按路由下钻。**生产通常两个都要**——stub_status 看连接层，日志转指标看业务层。

---

## 5. 手写一个 Exporter：Collector 模式

只有"自研中间件"才需要。模式固定三步（完整代码 `experiments/13_exporter.go`）：

```go
// ① 实现 Collector: Collect() 现查现填, Describe() 声明指标元信息
type myCollector struct{ ... }
func (c *myCollector) Describe(ch chan<- *desc)  { ch <- c.queueDepthDesc; ... }
func (c *myCollector) Collect(ch chan<- metric) {
    depth := c.client.QueryQueueDepth()   // 现问目标系统
    ch <- mustNewConstMetric(c.queueDepthDesc, gaugeValue, float64(depth))
}

// ② 注册到 Registry
reg.MustRegister(&myCollector{client: c})

// ③ 起 HTTP 端点
http.Handle("/metrics", metricsHandler(reg))
```

**四条工程纪律**（面试常问"写 exporter 要注意什么"）：

1. **`Collect()` 里不能有缓存，也不能太慢**：抓取超时会失败。查询超时要设（一般 1~3 秒），**超时就返回空而不是卡住**。
2. **目标不可用时怎么办**：返回该目标的 `up=0`（而不是整个 exporter 报错），这样只有这一个实例的告警触发，不会污染其他实例。
3. **集中式 exporter 用 `?target=` 参数**：Prometheus 侧用 `relabel_configs` 把 target 地址转成 `instance` 标签。
4. **不要在 `Collect()` 里创建新指标对象**：元信息（`Describe` 的输出）必须稳定——**每次抓取返回不同的指标名或 label 集会让 Prometheus 报错**。

---

## 6. blackbox：从外部探测

前面所有 Exporter 都是**从内部看**。blackbox_exporter 是**从外部探测**——模拟真实用户发请求：

```text
探 HTTP: 状态码、响应时间（分 DNS/连接/TLS/首字节/总耗时）、证书剩余天数
探 TCP:  端口通不通
探 ICMP: 机器活着吗
探 DNS:  解析是否正确
```

**WHY 从外部看是必要的**：内部指标全绿但用户访问不了的情况太多了——**DNS 挂了、证书过期了、负载均衡器摘了、防火墙规则错了**。这些故障**在服务自己的指标上完全不可见**（服务本身健康得很）。

**证书到期是经典用例**：`probe_ssl_earliest_cert_expiry < 7 天` 告警——**这属于 10 篇说的"预测性告警"**，用户还没受影响但再不动就出事。

---

## 7. 面试高频

**Q1：Exporter 是什么？和直接埋点的区别？**
Exporter 是适配器，把不会输出 Prometheus 格式的第三方系统（MySQL/nginx/操作系统）翻译成 `/metrics`。它独立进程、抓取时现查现答、不存数据；直埋是在自己代码里维护计数器（12 篇）。Exporter 存在的理由是不改动被监控方——与 sidecar 把横切能力做成邻居进程同源。

**Q2：node_exporter 和 mysqld_exporter 部署形态为什么不同？**
node_exporter 读本机 `/proc`，必须每机一个（DaemonSet）；mysqld_exporter 远程连 MySQL 查询，可以集中部署一个进程监控多个实例（`?target=` 区分）。取决于数据来源是"本机"还是"可远程查询"。

**Q3：MySQL 最该监控什么？**
连接池饱和度（`threads_connected / max_connections`，最常见故障）、主从延迟（读写分离架构的命门）、慢查询速率、缓冲池命中率低说明回源磁盘。

**Q4：Kafka 的头号指标是什么？为什么？**
消费 lag。lag 上涨 = 消费跟不上生产，在用户感知前有几小时预警窗口，是典型的先行指标。告警应按消费速率预测多久能追上（`predict_linear`），而不是固定条数阈值。

**Q5：Redis 的两个关键比率？**
内存使用率（到 100% 就开始淘汰 key，业务数据莫名消失）和命中率（掉到 90% 以下要查缓存穿透或大 key 淘汰）。主从场景加 `master_link_up`（0 = 复制断了）。

**Q6：nginx 的 stub_status 能看出什么？**
七个数字：活跃连接、accepts/handled/requests、Reading/Writing/Waiting。关键推导：`accepts - handled` = 被拒连接数（worker_connections 打满）；`Writing` 持续高 = 上游响应慢——这是 nginx 层关于后端变慢的最早信号。要按路由的 QPS 和状态码分布还得解析 access log。

**Q7：为什么需要 blackbox 这种外部探测？**
内部指标全绿但用户访问不了的情况很多：DNS 挂了、证书过期、LB 摘除、防火墙规则错——这些在服务自己的指标上完全不可见。外部探测模拟真实用户，证书到期告警是典型用例（预测性告警）。

**Q8：写 Exporter 要注意什么？**
四条：`Collect()` 必须快且有超时（超时返回空不卡住）、目标不可用时只让该目标 `up=0` 不污染其他实例、集中式用 `?target=` + relabel 转 `instance` 标签、`Collect()` 里绝不创建新的指标元信息（每次抓取返回不同的指标名会让 Prometheus 报错）。

---

本篇对应实验：experiments/13_exporter.go（手写 Collector 模式 Exporter：模拟一个内部系统的队列深度/吞吐指标，`Describe`/`Collect` 分离、目标不可用只让该实例 `up=0`、起 HTTP 端点供真实 Prometheus 抓取）
