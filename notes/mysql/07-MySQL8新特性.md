# MySQL 8.0 新特性

> **用到时知道有这东西，比死记更重要。** 这章是工具箱，遇到对应场景时来查。

---

## 窗口函数

不需要子查询就能做分组排名、环比同比。

```sql
-- 每个用户的订单按金额排名
SELECT user_id, amount,
  ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY amount DESC) AS rn,
  RANK()       OVER (PARTITION BY user_id ORDER BY amount DESC) AS rnk,
  DENSE_RANK() OVER (PARTITION BY user_id ORDER BY amount DESC) AS dense_rnk
FROM orders;

-- ROW_NUMBER: 1,2,3,4（无并列）
-- RANK:       1,1,3,4（并列后跳号）
-- DENSE_RANK: 1,1,2,3（并列后不跳号）
```

**环比/同比（LAG/LEAD）：**

```sql
-- 每月销售额与上月对比
SELECT month, revenue,
  LAG(revenue, 1)  OVER (ORDER BY month) AS last_month,
  LEAD(revenue, 1) OVER (ORDER BY month) AS next_month,
  revenue - LAG(revenue, 1) OVER (ORDER BY month) AS mom_diff
FROM monthly_sales;
```

**取分组内第一条（替代复杂子查询）：**

```sql
-- 每个用户金额最大的订单
SELECT * FROM (
  SELECT *, ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY amount DESC) AS rn
  FROM orders
) t WHERE rn = 1;
```

---

## CTE（公共表表达式）

用 `WITH` 子句给中间结果起名，替代嵌套子查询，可读性大幅提升。

```sql
-- 不用 CTE（嵌套难读）
SELECT * FROM (
  SELECT user_id, SUM(amount) AS total FROM orders GROUP BY user_id
) t WHERE total > 10000;

-- 用 CTE（清晰）
WITH user_totals AS (
  SELECT user_id, SUM(amount) AS total FROM orders GROUP BY user_id
)
SELECT * FROM user_totals WHERE total > 10000;
```

**递归 CTE（处理树形结构）：**

```sql
-- 查询组织架构树（员工 → 上级）
WITH RECURSIVE org AS (
  SELECT id, name, manager_id, 0 AS depth
  FROM employees WHERE manager_id IS NULL   -- 根节点
  UNION ALL
  SELECT e.id, e.name, e.manager_id, o.depth + 1
  FROM employees e JOIN org o ON e.manager_id = o.id
)
SELECT * FROM org ORDER BY depth, id;
```

---

## 自增列持久化

8.0 之前：自增值只存在内存，重启后从 `MAX(id)+1` 重新计算。
如果最大 id 的行被删了，重启后自增值会"回退"，可能复用已删除的 id，造成数据关联错乱。

8.0 之后：自增值写入 Redo Log，重启后从日志恢复，不会回退。

```sql
-- 8.0 之前的坑：
INSERT INTO t VALUES(10);  -- id=10
DELETE FROM t WHERE id=10;
-- 重启 MySQL
INSERT INTO t VALUES(NULL);  -- id 可能是 10（回退了），而不是 11
```

---

## utf8mb4 成为默认字符集

MySQL 的 `utf8` 实际上是 `utf8mb3`，只支持 3 字节 UTF-8，无法存储 Emoji 和部分生僻字（需要 4 字节）。

8.0 开始默认字符集改为 `utf8mb4`，真正支持完整 UTF-8。

```sql
-- 建表指定（或依赖默认值）
CREATE TABLE t (
  content TEXT
) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;

-- utf8mb4_0900_ai_ci：MySQL 8.0 默认排序规则
-- ai = accent insensitive（不区分重音）
-- ci = case insensitive（不区分大小写）
```

**迁移注意：** 老库从 utf8 迁到 utf8mb4 时，`VARCHAR(255)` 在 utf8 下是 765 字节，utf8mb4 下是 1020 字节，超过索引单列长度限制（767 字节），需要改用前缀索引或缩短列长度。
