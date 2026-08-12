# 事务与 MVCC

> **核心认知：事务的四个特性（ACID）不是独立的——原子性、隔离性、持久性共同保障一致性。MVCC 是用"空间换时间"的思路解决并发读写冲突：不加锁，用版本链让读者看到历史快照，写者互不干扰。**

---

## ACID

| 特性 | 含义 | InnoDB 实现 |
|------|------|-------------|
| 原子性 | 全部成功或全部回滚 | Undo Log |
| 一致性 | 数据完整性约束不被破坏 | 原子性 + 隔离性 + 持久性共同保障 |
| 隔离性 | 并发事务互不干扰 | MVCC + 锁 |
| 持久性 | 提交后永久保存 | Redo Log + Binlog 两阶段提交 |

---

## 隔离级别与并发问题

| 隔离级别 | 脏读 | 不可重复读 | 幻读 | 实现 |
|----------|------|-----------|------|------|
| READ UNCOMMITTED（读未提交） | ✗ | ✗ | ✗ | 直接读最新数据 |
| READ COMMITTED（读已提交） | ✓ | ✗ | ✗ | MVCC，每次查询生成新 ReadView |
| REPEATABLE READ（可重复读，默认） | ✓ | ✓ | 部分解决 | MVCC，事务首次查询时生成 ReadView，复用 |
| SERIALIZABLE（串行化） | ✓ | ✓ | ✓ | 所有操作加排他锁 |

**三种并发问题：**
- **脏读**：读到其他事务未提交的数据
- **不可重复读**：同一事务两次读同一行，结果不同（另一事务修改并提交了）
- **幻读**：同一事务两次相同条件查询，行数不同（另一事务插入/删除了）

InnoDB RR 通过 MVCC 解决快照读的幻读，通过 Next-Key Lock 解决当前读（FOR UPDATE）的幻读。

---

## MVCC 原理

每行数据有三个隐藏列：

```
| 数据列 | DB_TRX_ID | DB_ROLL_PTR | DB_ROW_ID |
           ↑              ↑              ↑
     最后修改的        指向 Undo Log    无主键时的
     事务 ID          里的旧版本       隐式主键
```

**版本链：** 每次修改把旧值写入 Undo Log，通过 `DB_ROLL_PTR` 串成链表，读时沿链往回找可见版本。

**ReadView（快照）：** 快照读时生成，包含：
- `creator_trx_id`：当前事务 ID
- `m_ids`：生成快照时仍活跃（未提交）的事务 ID 集合
- `min_trx_id`：m_ids 中最小值
- `max_trx_id`：下一个将分配的事务 ID

**可见性判断**（对版本链上某个版本的 `DB_TRX_ID` 判断）：

```
DB_TRX_ID == creator_trx_id          → 可见（自己改的）
DB_TRX_ID < min_trx_id               → 可见（已提交的老事务）
DB_TRX_ID >= max_trx_id              → 不可见（快照生成后才开启的事务）
min_trx_id <= DB_TRX_ID < max_trx_id → 在 m_ids 里则不可见，不在则可见
```

**用具体数字走一遍：**

```
场景：trx=10 做快照读，此时 trx=11 未提交，trx=12 已提交，trx=9 早已提交

ReadView：creator=10, m_ids=[10,11], min=10, max=13

版本链：
  DB_TRX_ID=11  balance=2000  ← 11 未提交
  DB_TRX_ID=12  balance=3000  ← 12 已提交
  DB_TRX_ID=9   balance=1000  ← 9 早已提交

判断：
  trx=11：在 m_ids → 不可见
  trx=12：min(10)<=12<max(13)，不在 m_ids → 可见 ✓  ← 读到 3000
  trx=9 ：9 < min(10) → 可见，但已被 trx=12 覆盖，取 trx=12 的值
```

**RC vs RR 的核心差异：**

| | ReadView 生成时机 | 效果 |
|-|-------------------|------|
| RC | 每次 SELECT 重新生成 | 能看到别人最新已提交的修改（不可重复读） |
| RR | 事务第一次 SELECT 时生成，整个事务复用 | 始终看到同一快照（可重复读） |

---

## 快照读 vs 当前读

| | 方式 | 加锁 | 读的数据 |
|-|------|------|---------|
| 快照读 | 普通 `SELECT` | 不加锁 | MVCC 版本链中的历史快照 |
| 当前读 | `SELECT ... FOR UPDATE` / `LOCK IN SHARE MODE` / `INSERT` / `UPDATE` / `DELETE` | 加锁 | 最新已提交数据 |

**RR 下幻读"部分解决"的真实含义：**

```sql
-- txA RR 级别：
SELECT COUNT(*) FROM orders WHERE user_id=1        -- 快照读，返回 5

-- txB 插入一行 user_id=1 并提交

SELECT COUNT(*) FROM orders WHERE user_id=1        -- 快照读，还是 5（MVCC 屏蔽）
SELECT COUNT(*) FROM orders WHERE user_id=1 FOR UPDATE  -- 当前读，返回 6！
```

同一事务混用快照读和当前读会看到不一致结果。FOR UPDATE 的幻读由 Next-Key Lock 防止（锁住间隙，阻塞其他事务插入）。

---

## 事务涉及的日志

一个 UPDATE 语句提交，背后四种日志各干什么：

```
UPDATE accounts SET balance = 2000 WHERE id = 1

1. 写 Undo Log（修改前）
   记录"如何撤销"：把 balance 从 2000 改回 1000
   存储位置：InnoDB 层，ibdata1 或独立 undo 表空间
   作用：① ROLLBACK 时执行 Undo 复原数据
         ② MVCC 版本链，DB_ROLL_PTR 指向这里

2. 改 Buffer Pool（内存）
   把数据页里 balance 改成 2000，标记为脏页，暂不落盘

3. 写 Redo Log —— prepare 阶段（WAL）
   记录"做了什么"：在 X 页 Y 偏移写入 2000
   顺序追加，极快；崩溃时用它重放恢复数据
   存储位置：InnoDB 层，ib_logfile0 / ib_logfile1（环形）

4. 写 Binlog（事务提交时）
   记录行变更（ROW 格式）或 SQL 原文（STATEMENT 格式）
   存储位置：Server 层，mysql-bin.000001 ...
   作用：主从复制 + 基于时间点的数据恢复

5. Redo Log —— commit 标记
   两阶段提交完成，事务正式提交
```

**四种日志对比：**

| 日志 | 层 | 记录内容 | 写入时机 | 作用 |
|------|-----|---------|---------|------|
| Undo Log | InnoDB | 如何撤销（旧值） | 修改前 | 回滚 + MVCC 版本链 |
| Redo Log | InnoDB | 做了什么（物理变更） | 事务执行中（WAL） | 崩溃恢复，保证持久性 |
| Binlog | Server | 行变更或 SQL 语句 | 事务提交时 | 主从复制 + 时间点恢复 |
| Relay Log | Server（从库） | 主库 Binlog 副本 | IO 线程接收时 | 从库重放用 |

**两阶段提交（2PC）为什么必须：**

```
若先提交 Redo 再写 Binlog，写 Binlog 时崩溃：
  主库有数据，Binlog 没记录 → 从库丢数据

若先写 Binlog 再提交 Redo，提交 Redo 时崩溃：
  Binlog 有记录，主库没数据 → 从库多数据

2PC 崩溃恢复规则：
  Redo 有 prepare + Binlog 存在 → 补 commit，提交
  Redo 有 prepare + Binlog 不存在 → 回滚
```

**长事务的危害：** Undo Log 被 purge 线程清理的前提是没有活跃事务在读老版本。长事务持有 ReadView，版本链无法清理，ibdata1 持续膨胀，读操作遍历链越来越慢。生产必须监控：

```sql
SELECT trx_id, trx_started, trx_state, trx_query
FROM information_schema.INNODB_TRX
WHERE TIME_TO_SEC(TIMEDIFF(NOW(), trx_started)) > 60;
```

> 详细展开见 [05-日志体系.md](05-日志体系.md)

---

## 实验

对应代码：[experiments/02_transaction_mvcc.go](experiments/02_transaction_mvcc.go)

```bash
go run ./experiments/ transaction
```

实验内容：
1. RR 级别可重复读：txB 修改并提交后，txA 第二次读结果不变
2. RC 级别不可重复读：txB 提交后，txA 第二次读能看到新值
3. 幻读：快照读（COUNT）不受新插入影响；FOR UPDATE 持 Gap Lock 阻止幻读
4. 快照读 vs 当前读：同一事务内混用，COUNT 结果不一致
5. 转账原子性：余额不足时事务回滚，总余额不变
