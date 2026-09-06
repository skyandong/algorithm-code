# Kafka 工程案例

> 学习笔记共 10 篇（`01-架构.md` ~ `10-选型对比.md`），每篇末尾有对应实验；另有 `面试一口答.md` 速查表。

## 快速开始

```bash
# 1. 启动 Kafka（KRaft 模式，无 ZooKeeper）
make up

# 2. 创建 Topic
make topics

# 3. 运行案例
make run-01   # 生产者基础
make run-02   # 消费者组（Ctrl+C 退出）
make run-03   # Exactly-Once
make run-04   # 死信队列（Ctrl+C 退出）
make run-05   # 消费积压监控（Ctrl+C 退出）
make run-06   # 多 Topic 流水线（Ctrl+C 退出）
make run-07   # 顺序性保证
make run-08   # 存储与读路径
make run-09   # 位移提交竞态 vs 滑动窗口
make run-10   # 分区倾斜观测
make run-11   # 集群健康巡检

# Kafka UI 可视化
open http://localhost:8080
```

## 统一运行入口（与 make 等价）

`make run-xx` 与下面的 `go run ./experiments/ xxx` 等价，实验代码本体都在 `experiments/` 目录内：

```bash
go run ./experiments/ basic     # = make run-01
go run ./experiments/ group     # = make run-02
go run ./experiments/ eos       # = make run-03
go run ./experiments/ dlq       # = make run-04
go run ./experiments/ lag       # = make run-05
go run ./experiments/ pipeline  # = make run-06
go run ./experiments/ ordering  # = make run-07
go run ./experiments/ storage   # = make run-08
go run ./experiments/ concurrent# = make run-09
go run ./experiments/ skew      # = make run-10
go run ./experiments/ health    # = make run-11
go run ./experiments/ all       # 运行可自动结束的案例（1、3、7~11）
```

## 案例说明

| 案例 | 代码 | 场景 | 核心知识点 |
|------|------|------|-----------|
| 1 | [experiments/01_producer_basic.go](experiments/01_producer_basic.go) | 同步/异步/批量发送 | acks、linger、回调 |
| 2 | [experiments/02_consumer_group.go](experiments/02_consumer_group.go) | 手动提交 offset | DisableAutoCommit、Rebalance 感知、优雅关闭 |
| 3 | [experiments/03_exactly_once.go](experiments/03_exactly_once.go) | 转账原子写 | 幂等生产者、事务、EOS |
| 4 | [experiments/04_dead_letter.go](experiments/04_dead_letter.go) | 订单消费失败处理 | 重试退避、DLQ、Header |
| 5 | [experiments/05_consumer_lag.go](experiments/05_consumer_lag.go) | 积压监控 | kadm、HW vs CommitOffset |
| 6 | [experiments/06_pipeline.go](experiments/06_pipeline.go) | 下单→支付→通知→审计 | 事件驱动、多消费者组、流水线 |
| 7 | [experiments/07_ordering.go](experiments/07_ordering.go) | 顺序性保证 | key 路由、分区内有序、消费端保序 |
| 8 | [experiments/08_storage.go](experiments/08_storage.go) | 存储与读路径 | start/end offset、时间戳定位、日志段 |
| 9 | [experiments/09_concurrent.go](experiments/09_concurrent.go) | 位移提交竞态 | 滑动窗口提交、offset 不超前 |
| 10 | [experiments/10_partition_skew.go](experiments/10_partition_skew.go) | 分区倾斜 | 热点 key、hash 路由、分区均衡 |
| 11 | [experiments/11_health.go](experiments/11_health.go) | 集群健康巡检 | broker/ISR/lag 三查 |

## 笔记导航

| 编号 | 笔记 | 主题 |
|------|------|------|
| 01 | [01-架构.md](01-架构.md) | 集群架构、副本、ISR、Leader 选举 |
| 02 | [02-消息保障.md](02-消息保障.md) | acks、幂等、事务、可靠性 |
| 03 | [03-原理与消费者.md](03-原理与消费者.md) | 消费者组、Rebalance、offset |
| 04 | [04-顺序性.md](04-顺序性.md) | 分区内有序、key 路由、消费端保序 |
| 05 | [05-存储与读路径.md](05-存储与读路径.md) | Segment、稀疏索引、retention/compact |
| 06 | [06-事务底层.md](06-事务底层.md) | Transaction Coordinator、LSO、EOS 边界 |
| 07 | [07-分区与容量设计.md](07-分区与容量设计.md) | 分区数估算、扩容代价、RF/min.insync |
| 08 | [08-消费并发模型.md](08-消费并发模型.md) | 三种消费模型、滑动窗口、max.poll.* |
| 09 | [09-监控与运维.md](09-监控与运维.md) | 分层指标、lag 告警、事故排查 |
| 10 | [10-选型对比.md](10-选型对比.md) | Kafka/RocketMQ/RabbitMQ/Pulsar 选型 |
| — | [面试一口答.md](面试一口答.md) | 考前一小时速查表 |

## 常用命令

```bash
make list-topics          # 列出所有 topic
make list-groups          # 列出所有消费者组
make lag GROUP=demo-group-1  # 查看指定组的积压
make down                 # 停止并清理
```
