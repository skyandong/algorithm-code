# MySQL 学习笔记

> 目标：系统掌握 MySQL 原理与工程实践，兼顾知识储备与工程能力提升。

## 目录

1. [索引体系](01-索引体系.md) — B+树、扇出、聚簇索引、覆盖索引、最左前缀、索引失效、ICP/MRR、隐藏索引、降序索引
2. [事务与 MVCC](02-事务与MVCC.md) — ACID、隔离级别、ReadView 可见性判断、快照读 vs 当前读、事务日志全流程
3. [锁机制](03-锁机制.md) — 行锁/间隙锁/临键锁、加锁规则、死锁复现与预防、SKIP LOCKED
4. [执行计划](04-执行计划.md) — EXPLAIN 字段、type 排序、Extra 关键值、EXPLAIN ANALYZE、JOIN 执行计划、慢 SQL 定位流程
5. [日志体系](05-日志体系.md) — Redo/Undo/Binlog、WAL、环形日志、两阶段提交、崩溃恢复、主从延迟
6. [Online DDL](06-Online-DDL.md) — MDL 锁雪崩、各版本演进、Row Log 膨胀、Instant DDL、原子 DDL、gh-ost vs pt-osc
7. [MySQL 8.0 新特性](07-MySQL8新特性.md) — 窗口函数、CTE、自增持久化、utf8mb4
8. [聚合查询练习](08-聚合查询练习.md) — GROUP BY、HAVING、窗口函数、ROLLUP、找茬错题对比（含 MySQL 8 真实报错）
9. [JOIN 原理与驱动表](09-JOIN原理与驱动表.md) — Nested Loop 三变体、驱动表选择、Hash Join、SQL 字段级注释
10. [WHERE 与 HAVING](10-WHERE与HAVING.md) — 执行顺序、聚合过滤边界、Error 1054 报错、别名特例、速查表
11. [面试一口答](面试一口答.md) — 考前速刷：30+ 条必须"张口就来"的核心点、高频三连问

---

## 实验代码

```bash
export MYSQL_DSN="root:password@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"

go run ./experiments/ index       # 索引：EXPLAIN 验证覆盖索引、最左前缀、失效场景、选择性
go run ./experiments/ transaction # 事务：RC/RR 可见性、幻读、快照读 vs 当前读、转账原子性
go run ./experiments/ lock        # 锁：X锁互斥、死锁复现、间隙锁、无索引表锁、SKIP LOCKED
go run ./experiments/ log         # 日志：Redo Log 写入量、Binlog 格式、慢查询日志
```

---

## 重点自测

**索引**
- [ ] B+ 树扇出是什么？为什么 3-4 层就能覆盖亿级数据？
- [ ] 聚簇索引和二级索引的叶子节点分别存什么？回表的完整路径是什么？
- [ ] 联合索引 `(a, b, c)`，`WHERE a=1 AND c=3` 为什么 c 失效？
- [ ] 索引列是 varchar，传数字参数，为什么索引失效？传反过来呢？
- [ ] ICP 和 MRR 分别解决什么问题？

**事务与 MVCC**
- [ ] RC 和 RR 的 ReadView 生成时机不同，导致什么行为差异？
- [ ] 快照读和当前读混用，同一事务会看到不一致结果吗？
- [ ] 长事务为什么会导致 Undo Log 膨胀、查询变慢？
- [ ] 一条 UPDATE 提交，Undo/Redo/Binlog 各在什么时机写入？

**锁**
- [ ] `UPDATE t SET x=1 WHERE non_index_col=5` 会锁多少行？
- [ ] 主键等值查询记录不存在时，加什么锁？
- [ ] 两个事务以相反顺序加锁为什么死锁？InnoDB 如何处理？
- [ ] SKIP LOCKED 适合什么场景？

**日志与 DDL**
- [ ] Redo Log 写满了会怎样？checkpoint 是什么？
- [ ] 两阶段提交解决什么问题？崩溃恢复时怎么对账？
- [ ] 执行 ALTER TABLE 前为什么要先查长事务？MDL 雪崩怎么发生的？
- [ ] gh-ost 和 pt-osc 的增量同步方式有什么本质区别？
