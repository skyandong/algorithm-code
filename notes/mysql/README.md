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
11. [分库分表](11-分库分表.md) — 拆分时机、分片键与分片算法、全局 ID、跨分片查询/JOIN/事务、双写迁移
12. [Buffer Pool 与内存](12-BufferPool与内存.md) — 改进版 LRU、脏页刷新、Change Buffer、Doublewrite、AHI
13. [主从复制与高可用](13-主从复制与高可用.md) — 复制流程、异步/半同步/MGR、GTID、并行复制、读写分离
14. [SQL 优化实战](14-SQL优化实战.md) — 深分页、COUNT、大表 DML、批量写入、IN vs EXISTS、optimizer_trace
15. [面试一口答](面试一口答.md) — 考前速刷：40+ 条必须"张口就来"的核心点、高频三连问

---

## 实验代码

```bash
export MYSQL_DSN="root:password@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"

go run ./experiments/ index       # 索引：EXPLAIN 验证覆盖索引、最左前缀、失效场景、选择性
go run ./experiments/ transaction # 事务：RC/RR 可见性、幻读、快照读 vs 当前读、转账原子性
go run ./experiments/ lock        # 锁：X锁互斥、死锁复现、间隙锁、无索引表锁、SKIP LOCKED
go run ./experiments/ log         # 日志：Redo Log 写入量、Binlog 格式、慢查询日志
go run ./experiments/ join        # JOIN：Index NLJ vs Hash Join、驱动表选择、STRAIGHT_JOIN
go run ./experiments/ aggregate   # 聚合：GROUP BY/窗口函数/ROLLUP，真实复现 Error 1054/1055
```

其他目录：
- `gorm/` — GORM 入门 demo（模型映射、AutoMigrate、CRUD）
- `ent/` — ent ORM 生成代码（schema 在 `ent/schema/`，需 `go get entgo.io/ent` 后使用，独立于实验代码）

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

**SQL 实战（JOIN / 聚合 / WHERE-HAVING）**
- [ ] 被驱动表 JOIN 列有索引和没索引，成本公式差在哪？8.0 无索引用什么兜底？
- [ ] "小表驱动大表"的"小"指什么？被驱动表没索引时 Extra 显示什么？
- [ ] 一对多 JOIN + `select *` + GROUP BY 为什么必挂？三种正确写法是什么？
- [ ] WHERE 里用聚合别名报什么错？HAVING 能用别名的依据是什么？
- [ ] 窗口函数的 ORDER BY 里为什么不能用 SELECT 别名？

**分库分表 / 内存 / 复制 / SQL 优化**
- [ ] 单表多大才需要分库分表？拆之前先穷尽哪些手段？
- [ ] 分片键怎么选？哈希取模扩容难在哪，翻倍扩容怎么解决？
- [ ] UUID 为什么不适合做分库分表后的主键？雪花算法 64 位怎么分？
- [ ] InnoDB 的 LRU 和教科书 LRU 差在哪三件事？怎么防全表扫描污染？
- [ ] Change Buffer 为什么只对非唯一二级索引生效？
- [ ] Doublewrite 解决什么问题？为什么 Redo Log 解决不了？
- [ ] 半同步 AFTER_COMMIT 和 AFTER_SYNC 谁有幻读风险？
- [ ] GTID 相比位点复制好在哪？并行复制三代演进？
- [ ] 深分页 LIMIT 1000000,10 为什么慢？延迟关联和游标分页各治什么？
- [ ] InnoDB COUNT(*) 为什么慢？大表 DELETE 为什么要分批？
