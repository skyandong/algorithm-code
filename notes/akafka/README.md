# Kafka 工程案例

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

## 案例说明

| 案例 | 场景 | 核心知识点 |
|------|------|-----------|
| 01-producer-basic | 同步/异步/批量发送 | acks、linger、回调 |
| 02-consumer-group | 手动提交 offset | DisableAutoCommit、Rebalance 感知、优雅关闭 |
| 03-exactly-once | 转账原子写 | 幂等生产者、事务、EOS |
| 04-dead-letter | 订单消费失败处理 | 重试退避、DLQ、Header |
| 05-consumer-lag | 积压监控 | kadm、HW vs CommitOffset |
| 06-pipeline | 下单→支付→通知→审计 | 事件驱动、多消费者组、流水线 |

## 常用命令

```bash
make list-topics          # 列出所有 topic
make list-groups          # 列出所有消费者组
make lag GROUP=demo-group-1  # 查看指定组的积压
make down                 # 停止并清理
```
