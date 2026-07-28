# Redis 学习笔记

> 目标:系统掌握 Redis 原理与工程实践,兼顾知识储备与工程能力提升。

## 目录

1. [为什么 Redis 这么快](01-为什么这么快.md)
2. [数据类型与底层结构](02-数据类型与底层结构.md)
3. [持久化](03-持久化.md)
4. [高可用](04-高可用.md)
5. [缓存常见问题](05-缓存常见问题.md)
6. [内存管理与淘汰](06-内存管理与淘汰.md)
7. [事务 / Lua / Pipeline](07-事务-Lua-Pipeline.md)
8. [分布式锁](08-分布式锁.md)
9. [生产排查与运维](09-生产排查与运维.md)
10. [其他常见追问](10-其他追问.md)

## 重点回顾(自测)

- [ ] 分布式锁完整链路(SET NX → Lua 释放 → Redisson watchdog → Redlock)
- [ ] 内存淘汰策略 + 近似 LRU/LFU 原理
- [ ] 过期机制(惰性 + 定期)+ 大 Key 异步删除
- [ ] 事务/Lua/Pipeline 三者区别
- [ ] 主从 PSYNC 全量 + 部分重同步
- [ ] 哨兵故障转移流程 + 防脑裂
- [ ] 集群哈希槽 + ASK/MOVED + hash tag
- [ ] bgsave 的 COW + AOF 重写
- [ ] bigkey/hotkey/SCAN/slowlog 生产排查
