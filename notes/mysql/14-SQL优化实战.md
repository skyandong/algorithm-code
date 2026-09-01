# SQL 优化实战

> **核心认知：慢 SQL 的根因 90% 是"扫了不该扫的行"或"做了不该做的额外工作（回表/排序/临时表）"。套路化的优化三板斧：少扫（索引）、少回（覆盖）、少算（先过滤再聚合）。本章是前面所有章节的工程落地。**

> 排查流程见 [04-执行计划.md](04-执行计划.md)：慢日志定位 → EXPLAIN → EXPLAIN ANALYZE → 对症下药。本章讲对症下药的具体套路。

---

## 一、深分页优化（LIMIT 1000000, 10）

**为什么慢：** MySQL 要取出前 1000010 行（二级索引场景还要逐行回表），再丢掉前 100 万行。扫描量和偏移量成正比。

```sql
-- 慢：偏移 100 万，扫 100 万行 + 全部回表
SELECT * FROM orders WHERE user_id = 1 ORDER BY id LIMIT 1000000, 10;
```

### 优化一：延迟关联（覆盖索引先拿 id）

```sql
-- 子查询只走覆盖索引拿主键（不回表），再用 10 个 id 回表取整行
SELECT o.* FROM orders o
JOIN (SELECT id FROM orders WHERE user_id = 1 ORDER BY id LIMIT 1000000, 10) t
  ON o.id = t.id;
```

前提：`(user_id, id)` 能构成覆盖索引（InnoDB 二级索引叶子天然带主键，`idx(user_id)` 即可）。
效果：扫 100 万行索引但不回表，最后只回表 10 次——快 5~10 倍，但仍是 O(offset)。

### 优化二：游标 / seek 分页（真正的 O(10)）

```sql
-- 记住上一页最后一条的 id，下一页从它之后取
SELECT * FROM orders WHERE user_id = 1 AND id > :last_max_id ORDER BY id LIMIT 10;
```

- 利用索引直接定位起点，与页码无关，第 1 页和第 10 万页一样快
- **限制：** 只支持"下一页"式翻页（App 信息流天然适合）；跳页需要换算或退化
- 排序键必须唯二（`ORDER BY id`），否则用 `(created_at, id) > (:last_time, :last_id)` 组合游标

**面试标准答案：** 延迟关联治标（少回表），游标分页治本（不扫偏移），产品能接受连续翻页就用游标。

---

## 二、COUNT 优化

**为什么慢：** InnoDB 不存总行数（MVCC 下每个事务看到的行数不同），`COUNT(*)` 要真实扫描。MyISAM 无 WHERE 时直接返回存储值，这是"为什么 InnoDB COUNT 慢"的答案。

| 写法 | 行为 |
|------|------|
| `COUNT(*)` / `COUNT(1)` | 扫描，优化器自动选**最小的索引**，NULL 也算 |
| `COUNT(id)` | id 非空，语义等同 COUNT(*)，执行计划基本相同（优化器同样选最小可用索引） |
| `COUNT(col)` | 忽略 NULL，语义不同 |

**优化手段：**

```sql
-- ① 只要近似值：EXPLAIN 的 rows 估算（零成本，误差可到 ±50%）
EXPLAIN SELECT * FROM orders;

-- ② 覆盖索引扫最小索引
SELECT COUNT(*) FROM orders FORCE INDEX(idx_smallest);

-- ③ 精确值：计数表 + 同事务更新，或 Redis 计数（注意一致性）

-- ④ 带条件 COUNT 拆分：COUNT(status=0 OR NULL) 替代多条查询
SELECT COUNT(status = 0 OR NULL) AS s0, COUNT(status = 1 OR NULL) AS s1 FROM orders;
```

---

## 三、大表 DML（安全删除/更新）

**一条 DELETE 影响百万行的三大危害：** 长事务（Undo 膨胀、MDL 持续）、主从延迟（从库重放同样久）、锁持有过久。还可能一次刷出巨量脏页。

```sql
-- 错误：一条 SQL 删 500 万行
DELETE FROM logs WHERE created_at < '2023-01-01';

-- 正确：分批 + 主键游标，每批几千行，批间 sleep 给从库追的时间
DELETE FROM logs
WHERE id <= (SELECT max_id FROM (
    SELECT id FROM logs WHERE created_at < '2023-01-01' ORDER BY id LIMIT 5000
) t)
AND created_at < '2023-01-01';
-- 循环执行直到 affected rows = 0

-- 更彻底：归档场景用 pt-archiver（边删边归档，自动限速）
pt-archiver --source h=host,D=db,t=logs --where "created_at < '2023-01-01'" \
            --dest h=archive_host,D=archive,t=logs --limit 5000 --commit-each
```

要点：`created_at` 上必须有索引（否则每批都是全表扫）；批量大小 1000~5000；大 UPDATE 同理。**整表清空用 `TRUNCATE`（DDL，瞬间）不用 `DELETE`。**

---

## 四、批量写入优化

```sql
-- 慢：1000 个事务，每事务 fsync 一次（双一配置 = 1000 次磁盘等待）
INSERT INTO t VALUES (1);
INSERT INTO t VALUES (2);
...

-- 快：一条 multi-values SQL，一个事务一次 fsync
INSERT INTO t VALUES (1), (2), ..., (1000);
```

- 批量大小受 `max_allowed_packet`（默认 64MB）限制，几百到几千行为宜
- 超大导入：`LOAD DATA INFILE` 最快；**先导完数据、再建二级索引**（空表时边插边维护索引最慢，导完后一次性建索引快得多）
- GORM：`db.CreateInBatches(rows, 1000)`，避免 `Create` 单条循环

---

## 五、IN vs EXISTS

```sql
-- IN：外层大表扫描，子查询结果做缓存/驱动
SELECT * FROM orders WHERE user_id IN (SELECT id FROM users WHERE vip = 1);
-- EXISTS：外层表逐行拿 user_id 去探测子查询
SELECT * FROM orders o WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = o.user_id AND u.vip = 1);
```

- 老经验："小表驱动大表"：子查询结果小用 IN，外层表小用 EXISTS
- **MySQL 5.6+ 会把 IN 子查询改写为 semijoin（半连接）**，两种写法多数场景执行计划相同——面试答出 semijoin 改写是加分项
- `NOT IN` 含 NULL 的坑：子查询结果有 NULL 时 `NOT IN` 永远返回空集，用 `NOT EXISTS`

---

## 六、高频小优化清单

| 场景 | 做法 |
|------|------|
| `OR` 连接同索引列 | 优化器可用 index_merge，但改成 `UNION ALL` 往往更稳 |
| `ORDER BY` 随机排序 | `ORDER BY RAND()` 全表 + filesort，改为随机主键区间抽取 |
| 前端只需存在性 | `SELECT 1 ... LIMIT 1` 代替 `COUNT(*)`，找到一行即停 |
| 一次查询多聚合 | 多次扫描合并为一次扫描多个聚合函数 |
| 子查询做过滤 | 5.6+ 优先 semijoin；派生表（FROM 子查询）注意物化成本 |
| `INSERT ... SELECT` 大量行 | 拆批 + 低峰执行，防止主从延迟 |
| JSON 字段查询 | 8.0.13+ 可对 JSON 表达式直接建**函数索引**，8.0.17+ 有 MEMBER OF 多值索引；生成列是兼容性最好的方案 |

```sql
-- 生成列给 JSON 字段建索引
ALTER TABLE events ADD COLUMN event_type VARCHAR(32)
  GENERATED ALWAYS AS (JSON_UNQUOTE(JSON_EXTRACT(data, '$.type'))) STORED,
  ADD INDEX idx_event_type(event_type);
```

---

## 七、优化器行为诊断（进阶）

```sql
-- 看优化器为什么选这个计划（索引为什么没选）
SET optimizer_trace = 'enabled=on';
SELECT ...;  -- 执行后查 trace
SELECT * FROM information_schema.OPTIMIZER_TRACE\G

-- 树形执行计划（8.0.16+，比表格直观）
EXPLAIN FORMAT=TREE SELECT ...;

-- 真实执行 + 每算子耗时
EXPLAIN ANALYZE SELECT ...;
```

三件套用法：EXPLAIN 看计划 → FORMAT=TREE 看算子结构 → ANALYZE 看真实耗时，optimizer_trace 解释"为什么不走那个索引"。

---

## 八、面试一句话总结

- 深分页慢在"扫了偏移量那么多行再丢掉"：延迟关联（覆盖索引拿 id，少回表）治标，**游标分页 `WHERE id > last_id` 治本**（O(1) 但只能连续翻页）。
- InnoDB 不存行数（MVCC 每事务可见行数不同），COUNT(*) 选最小索引扫；近似值用 EXPLAIN rows，精确值用计数表/Redis。
- 大表 DELETE 三害：长事务 Undo 膨胀、主从延迟、锁持久的久。解法：主键游标分批 + 限速，或 pt-archiver。
- 批量插入用 multi-values / CreateInBatches，一个事务一次 fsync；整表清空用 TRUNCATE。
- IN/EXISTS 在 5.6+ 多被改写为 semijoin，执行计划趋同；`NOT IN` 遇 NULL 返回空集，用 `NOT EXISTS`。
- JSON 字段查询用生成列 + 索引；诊断三件套：EXPLAIN / FORMAT=TREE / ANALYZE + optimizer_trace。
