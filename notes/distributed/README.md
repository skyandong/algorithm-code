# distributed 学习笔记

> 目标：系统掌握分布式理论——CAP/PACELC、Raft 共识、分布式事务、锁与协调、复制分片、逻辑时钟、故障检测。所有结论对已有实践（MySQL/Redis/Kafka/nginx 笔记）做理论收口：**实践先行、理论串线**——学过的主从复制、哨兵、ISR 在这里找到统一框架。全部实验可运行：quorum 可见性、选主模拟、本地消息表闭环、锁攻击时间线、一致性哈希迁移量、φ-accrual、gossip 收敛。
> **主线认知：** 分布式只有两个根本困难——**网络不可靠**（消息会丢、会延迟：所以有 CAP 的分区、故障检测的不确定）和**时钟不可信**（所以有 term/向量时钟/雪花回拨）。所有协议都是围绕这两点的工程应对。

## 目录

1. [CAP / BASE / PACELC](01-CAP-BASE-PACELC.md) — 一致性谱系、分区时的选择、真实系统定位表
2. [共识：Raft](02-共识：Raft.md) — 选主/日志复制/安全性三分解、term 逻辑时钟、选举限制
3. [分布式事务](03-分布式事务.md) — 2PC 之死、TCC/Saga、本地消息表（最常用）
4. [锁与协调实践](04-锁与协调实践.md) — etcd/Redis/ZK 对比、fencing、Redlock 之争、watch
5. [复制与分片](05-复制与分片.md) — 主从/多主/无主、range vs hash、一致性哈希与虚拟节点
6. [时钟与顺序](06-时钟与顺序.md) — LWW 丢写、Lamport/向量时钟、HLC、雪花 ID 回拨
7. [故障检测](07-故障检测.md) — 心跳权衡、φ-accrual、gossip、防脑裂三道闸
8. [面试一口答](面试一口答.md) — 考前速刷：高频问题「张口就来」

## 重点回顾(自测)

- [ ] CAP 不是三选二：P 是前提，分区时才二选一；PACELC 补上平时的 L vs C
- [ ] 一致性谱系：线性 > 顺序 > 因果 > 读己之写 > 最终——按读写路径选不是按系统选
- [ ] 已有系统的定位：MySQL 主从 AP/EL、Redis 哨兵 AP/EL、Kafka acks=all CP/EC、etcd CP/EC
- [ ] Raft 三分解：选主（随机超时+term+过半）、复制（过半确认+连续性检查）、安全（选举限制）
- [ ] term 是逻辑时钟；旧 Leader 见更大 term 自动退位；pre-vote 防扰动
- [ ] 选举限制 → 新 Leader 必持有全部已提交日志 → 提交不可逆
- [ ] 2PC 三伤：跨网持锁、协调者单点、脑裂；实践全走最终一致路线
- [ ] 本地消息表：同库同事务消灭「业务成功消息丢」；投递重试+死信；消费端幂等
- [ ] 锁正确性 = 存储 + fencing：etcd revision 天然 fencing；Redis 锁的 AP 上限
- [ ] TTL 攻击（进程停顿）对所有超时锁成立 → 资源侧强制校验 fencing token
- [ ] 一致性哈希：加节点只挪 1/N；虚拟节点 100~200/节点；哈希函数要雪崩好的（fnv 会聚集）
- [ ] LWW 按物理时间戳会丢写；向量时钟能判并发 → 触发裁决而非静默丢
- [ ] 雪花 41+10+12；时钟回拨：小等大拒；号段模式时钟无关
- [ ] φ-accrual 输出怀疑度而非 0/1；gossip ln(N) 轮收敛；suspect→间接探测→dead 三道闸

## 跑实验

```bash
cd notes/distributed
go run ./experiments/ all      # 全部实验
go run ./experiments/ raft     # 单跑：02 篇
# 可用名: cap|raft|localmsg|locks|sharding|clock|phi

go run -race ./experiments/ all  # 并发实验必开
```

**实验数据锚点**（跑出来应接近这些值，偏差大说明改坏了）

| 实验 | 理论值 |
|---|---|
| quorum W=3 R=3 (N=5) | 旧读 0 次；W+R≤N 时旧读成常态 |
| Raft 随机超时 | ~1.8% 轮分裂（无随机=100%） |
| 一致性哈希 4→5 节点 | 迁移 ~15-20%（取模 ~80%） |
| vnode 200/节点 | 不均衡 ~1.1x（无 vnode 可达 20x+） |
| gossip N=1000 k=3 | ~8 轮收敛（ln(1000)≈6.9） |

**文件说明**

| 文件 | 内容 |
|------|------|
| `experiments/01-07_*.go` | 每篇对应的可运行验证，零外部依赖（全内存模拟） |
| `experiments/06_clock.go` | 雪花 ID 生成器 + 回拨拒绝策略 + 可注入墙钟 |
| `go.mod` | 独立 module `adistributed`（Go 1.26） |

## 与其他模块的衔接

- `notes/mysql/13` — 主从复制/半同步：01 篇 PACELC 表、02 篇 Raft 复制的对照组
- `notes/mysql/11` — 分库分表：05 篇分片理论的数据库形态
- `notes/redis/04` / `08` — 哨兵选主、分布式锁看门狗：02/04 篇的理论对照
- `notes/akafka` — ISR 与 acks：01/02 篇的「过半确认」在 MQ 的实现
- `notes/design-pattern/04` — 状态机：Saga 编排引擎的本质
- `engineering/consistenthash` — 一致性哈希的工程实现（带 design.md 与测试）
- `notes/design-pattern/07` — 重试边界：本地消息表投递器的策略层
