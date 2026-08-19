# Kafka 工程案例

> 学习笔记在 `01-架构.md` / `02-消息保障.md` / `03-原理与消费者.md`，每篇末尾有对应实验。

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
go run ./experiments/ all       # 运行可自动结束的案例（1 和 3）
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

## 常用命令

```bash
make list-topics          # 列出所有 topic
make list-groups          # 列出所有消费者组
make lag GROUP=demo-group-1  # 查看指定组的积压
make down                 # 停止并清理
```
