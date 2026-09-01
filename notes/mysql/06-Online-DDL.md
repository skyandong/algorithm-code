# Online DDL

> **核心认知：DDL 的本质风险不是"锁表多久"，而是 MDL 写锁等待期间造成的连接雪崩。理解 MDL 机制，是安全执行大表 DDL 的前提。**

---

## 各版本演进

| 版本 | 方式 | 特点 |
|------|------|------|
| 5.5 及之前 | Copy Table | 锁全表，复制到临时表，期间 DML 阻塞 |
| 5.6 | Online DDL（部分） | `ALGORITHM=INPLACE`，大部分操作无需拷贝表 |
| 5.7 | 增强 | `RENAME INDEX` 无需重建表；更多 DDL 支持 INPLACE |
| 8.0 | Instant DDL | 加列到末尾等操作只改元数据，瞬间完成；VARCHAR 扩容支持 INPLACE（长度字节数不变时仅改元数据） |

---

## MDL 锁（Metadata Lock）

MDL 是 **Server 层的元数据锁**，保护表结构，与 InnoDB 行锁无关。

```
任何 SQL 访问表：自动加 MDL 读锁（事务结束才释放）
DDL 修改表结构：需要 MDL 写锁（排他）
```

**MDL 等待为什么会引发雪崩：**

```
txA：普通 SELECT，持有 MDL 读锁，事务未提交（长事务）

ALTER TABLE：申请 MDL 写锁，被 txA 阻塞，进入等待队列

后续所有 SELECT/INSERT/UPDATE：
  申请 MDL 读锁，但写锁请求在队列中，读锁被阻塞
  → 连接堆积 → 连接数耗尽 → 数据库雪崩
```

**执行 DDL 前必须确认：**

```sql
-- 确认无长事务（持有 MDL 读锁）
SELECT trx_id, trx_started, trx_query
FROM information_schema.INNODB_TRX
WHERE TIME_TO_SEC(TIMEDIFF(NOW(), trx_started)) > 10;

-- 确认无 MDL 等待（需先开启 instrument：UPDATE performance_schema.setup_instruments
-- SET enabled='YES' WHERE name='wait/lock/metadata/sql/mdl'，MDL instrument 默认关闭）
SELECT * FROM performance_schema.metadata_locks
WHERE OBJECT_SCHEMA = 'mydb' AND OBJECT_NAME = 'orders';
```

---

## Online DDL 执行流程（5.6+）

```
1. 获取 MDL 排他锁（极短，阻塞读写）
2. 降级为 MDL 读锁（允许 DML 继续）
3. 创建新临时表（按目标结构）
4. 逐行复制数据到新表
5. 将 DDL 期间的 DML 变更（Row Log）应用到新表
6. 升级 MDL 为排他锁（短暂阻塞）
7. 原子 RENAME：新表替换旧表
8. 释放 MDL 锁
```

关键参数：
```sql
ALTER TABLE t ADD COLUMN remark varchar(256),
ALGORITHM=INPLACE,  -- INPLACE：原地修改；COPY：强制拷表
LOCK=NONE;          -- NONE：不加锁；SHARED：允许读；EXCLUSIVE：独占
```

**Row Log 膨胀风险：**

步骤 4 复制数据期间，原表 DML 变更记录在 Row Log 里，步骤 5 再应用到新表。

```
Row Log 上限：innodb_online_alter_log_max_size（默认 128MB）

超出上限 → DDL 报错 ERROR 1799，前面复制的数据全部作废，从头来过
```

应对：选业务低峰、调大 `innodb_online_alter_log_max_size`，或改用 gh-ost（可限速）。

---

## MySQL 8.0 Instant DDL

只修改元数据（Data Dictionary），不触碰实际数据行，瞬间完成：

```sql
ALTER TABLE t ADD COLUMN new_col int DEFAULT 0, ALGORITHM=INSTANT;
```

**限制：**
- 新列必须加到表的最后（8.0.29+ 支持任意位置）
- 表不能有全文索引
- 压缩格式表（ROW_FORMAT=COMPRESSED）不支持

---

## 原子 DDL（8.0+）

8.0 之前，DDL 操作不是原子的：`DROP TABLE t1, t2` 如果中途崩溃，可能 t1 删了 t2 没删，元数据和实际文件不一致。

8.0 引入原子 DDL，所有 DDL 操作要么完全成功，要么完全回滚，不会留下半成品。

```sql
DROP TABLE t1, t2;  -- 8.0+：要么全删，要么全不删，崩溃不留垃圾
```

实现原理：DDL 操作也写入 Redo Log，崩溃恢复时和普通事务一样对账。

---

## 大表加索引最佳实践

1. **首选 Instant DDL**（8.0：加列、RENAME COLUMN、ALTER COLUMN SET/DROP DEFAULT 等纯元数据操作；8.0.29+ 还支持 INSTANT DROP COLUMN 和任意位置加列）
2. **评估 Online DDL 可行性**：`ALGORITHM=INPLACE, LOCK=NONE`，在业务低峰执行
3. **超大表（>5000万行）用 gh-ost / pt-osc**：

```bash
# gh-ost（GitHub 开源，通过 Binlog 同步增量，对主库压力更小）
gh-ost \
  --host=127.0.0.1 --port=3306 \
  --database=mydb --table=orders \
  --alter="ADD INDEX idx_user_id(user_id)" \
  --execute

# pt-online-schema-change（Percona，通过触发器同步增量）
pt-online-schema-change \
  --alter="ADD INDEX idx_user_id(user_id)" \
  D=mydb,t=orders \
  --execute
```

**gh-ost vs pt-osc 核心差异：**

| | pt-osc | gh-ost |
|-|--------|--------|
| 增量同步方式 | 触发器（INSERT/UPDATE/DELETE 各一个） | 模拟从库，读主库 Binlog 流 |
| 原表侵入 | 有触发器，每次 DML 有额外开销 | 零侵入，不加触发器 |
| 限速能力 | 有，但较粗 | `--max-load` 监控主库负载，超阈值自动暂停 |
| 已有触发器 | 不支持 | 支持 |
| 可演练 | `--dry-run`（建影子表执行 alter，不建触发器不拷数据） | `--dry-run` 不真正执行 |
| 生产推荐 | 次选 | **首选** |

两者核心思路相同：
1. 创建影子表（目标结构）
2. 全量复制存量数据 + 增量同步 DDL 期间的 DML
3. 数据一致后原子 RENAME 切换
4. 可随时暂停/回滚，不影响业务
