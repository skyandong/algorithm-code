# JOIN 原理与驱动表

> **核心认知：MySQL 的 JOIN 本质是"嵌套循环"——取一行驱动表，去被驱动表找匹配。驱动表越小、被驱动表 JOIN 列越有索引，越快。"小表连大表"是经验说法，准确讲是"扫描成本小的表当驱动表"。**

---

## 一、Nested Loop Join 三种变体

### 1. Simple Nested Loop Join（朴素版，实际不用）

```sql
SELECT c.class_name,        -- 班级名称(来自驱动表)
       s.student_name       -- 学生姓名(来自被驱动表)
FROM class_info c           -- 班级表(驱动表,全表扫 N 行)
JOIN student_score s        -- 成绩表(被驱动表,被查 N 次,每次全表扫)
  ON s.class_id = c.class_id; -- JOIN 条件(class_id 无索引 → 每次全表扫 M 行)
```

成本：`N + N × M`。基本不用。

### 2. Index Nested Loop Join（被驱动表 JOIN 列有索引，最理想）

```sql
SELECT c.class_name,        -- 班级名称(来自驱动表)
       s.student_name       -- 学生姓名(来自被驱动表)
FROM class_info c           -- 班级表(驱动表,全表扫 N 行)
JOIN student_score s        -- 成绩表(被驱动表,被查 N 次,每次走索引)
  ON s.class_id = c.class_id; -- JOIN 列(有索引 → 每次查询 O(log M))
-- 前提: student_score.class_id 上有索引
```

成本：`N + N × log M`。**这是最理想的——所以"被驱动表 JOIN 列有索引"比"驱动表大小"更关键。**

### 3. Block Nested Loop Join（无索引兜底，用 join_buffer）

```sql
SELECT c.class_name,        -- 班级名称(来自驱动表)
       s.student_name       -- 学生姓名(来自被驱动表)
FROM class_info c           -- 班级表(驱动表,分批塞进 join_buffer)
JOIN student_score s        -- 成绩表(被驱动表,全表扫,但只扫 1 次/批)
  ON s.class_id = c.class_id; -- JOIN 列(无索引 → BNLJ 兜底)
-- EXPLAIN 的 Extra 会出现 "Using join buffer (Block Nested Loop)"
```

成本：`N + N/batch × M`（batch = join_buffer 能装的行数）。比朴素版好，但仍贵。

---

## 二、驱动表怎么选

```sql
EXPLAIN
SELECT g.grade_name,        -- 年级名称(来自驱动表)
       c.class_name,        -- 班级名称(来自被驱动表1)
       s.student_name       -- 学生姓名(来自被驱动表2)
FROM grade g                -- 年级表(过滤后行数最少 → 优化器选它当驱动表)
JOIN class_info c           -- 班级表(被驱动表,JOIN 列 grade_id)
  ON c.grade_id = g.grade_id   -- JOIN 条件(被驱动表 JOIN 列,需有索引)
JOIN student_score s        -- 成绩表(被驱动表2,JOIN 列 class_id)
  ON s.class_id = c.class_id;  -- JOIN 条件(被驱动表 JOIN 列,需有索引)
-- 优化器看 rows 估算 + 索引,自动选扫描成本最小的当驱动表
-- 书写顺序 ≠ 执行顺序;MySQL 不保证按你写的顺序 JOIN
```

**"小表"的准确定义**：不是表本身行数小，是 **WHERE 过滤后进入 JOIN 的行数小**。

```sql
SELECT a.uid,               -- 用户ID(来自驱动表)
       b.order_id           -- 订单ID(来自被驱动表)
FROM users a                -- 用户表(WHERE 过滤后剩 100 行 → 驱动表)
JOIN orders b               -- 订单表(100 万行 → 被驱动表)
  ON b.uid = a.uid          -- JOIN 条件(被驱动表 JOIN 列 uid,需有索引)
WHERE a.status = 1;        -- 过滤条件(让 A 变"小",从而当驱动表)
```

---

## 三、Hash Join（8.0.18+，无索引场景的救星）

```sql
SELECT c.class_name,        -- 班级名称(来自建哈希的小表)
       s.student_name       -- 学生姓名(来自探测的大表)
FROM class_info c           -- 班级表(小表,建哈希表,放内存)
JOIN student_score s        -- 成绩表(大表,扫一遍,探测哈希表)
  ON s.class_id = c.class_id; -- JOIN 列(无索引也能用 Hash Join)
-- 成本 O(N+M),比 BNLJ 快很多
-- MySQL 8.0.18+ 自动选,替代了 BNLJ(8.0.20 起彻底移除 BNLJ)
-- EXPLAIN 的 Extra 会出现 "Using join buffer (hash join)"（裸的 "hash join" 节点名只在 FORMAT=TREE/ANALYZE 树形输出中出现）
```

Hash Join 让"小表建哈希"成为关键 → 建哈希那方还是越小越好，但整体成本从 `O(N×M)` 降到 `O(N+M)`，**驱动表大小的权重下降**。

---

## 四、强制驱动表顺序（STRAIGHT_JOIN）

优化器偶尔会判断失误，可用 `STRAIGHT_JOIN` 强制左表当驱动表（慎用）：

```sql
SELECT STRAIGHT_JOIN        -- 强制按书写顺序选驱动表
       c.class_name,        -- 班级名称
       s.student_name       -- 学生姓名
FROM class_info c           -- 班级表(强制当驱动表)
JOIN student_score s        -- 成绩表(强制当被驱动表)
  ON s.class_id = c.class_id; -- JOIN 条件
-- 仅在确认优化器选错时用;数据变化后可能反而变慢
```

---

## 五、坑：`select *` + GROUP BY 在严格模式下必挂

功能依赖能省略 GROUP BY 列的规则,只在"GROUP BY 列能唯一决定 SELECT 列"时成立。反过来在一对多 JOIN 上做 GROUP BY,会直接报 Error 1055。

### 反面案例

```sql
SELECT *                       -- 展开所有列(含 s 表的 sid/student_name/score)
FROM class_info c              -- 班级表(驱动表)
LEFT JOIN student_score s      -- 成绩表(被驱动表,一个班多行)
  ON s.class_id = c.class_id;  -- JOIN 条件
GROUP BY c.class_id;           -- 按班级分组(每班多行 → 压成一行)
```

**真实报错：**

```
1055 - Expression #4 of SELECT list is not in GROUP BY clause
and contains nonaggregated column 'test.s.sid'
which is not functionally dependent on columns in GROUP BY clause
this is incompatible with sql_mode=only_full_group_by
```

### 为什么挂

JOIN 后每班**多行**(每个学生一行),`GROUP BY c.class_id` 把每班多行压成一行,但 `select *` 要展示 `s.sid` / `s.student_name` / `s.score`——一个班有多个值,**MySQL 不知道返回哪个**。

`select *` 展开后的列与功能依赖关系:

| # | 列 | 来源 | 能被 `c.class_id` 决定? | 说明 |
|---|---|---|---|---|
| 1 | c.class_id | class_info | ✓ | GROUP BY 本身 |
| 2 | c.grade_id | class_info | ✓ | class_id 是主键,决定它 |
| 3 | c.class_name | class_info | ✓ | class_id 是主键,决定它 |
| 4 | **s.sid** | student_score | ✗ | 一个班多个学生,决定不了 |
| 5 | s.class_id | student_score | ✗ | 同上 |
| 6 | s.student_name | student_score | ✗ | 同上 |
| 7 | s.score | student_score | ✗ | 同上 |

第 4 列 `s.sid` 是第一个无法被功能依赖的列,所以报它。

### 对比习题 1 的正向情况

习题 1 的 `GROUP BY c.class_id` 能省略 `c.class_name`,因为 `class_id → class_name` 是"一对一"(class_id 是主键)。这里方向反了——`c.class_id → s.sid` 是"一对多",决定不了,所以挂。

### 怎么改

**情况 1:要每个班的信息(不要学生明细)——用聚合:**

```sql
SELECT c.class_id,            -- 班级ID
       c.class_name,          -- 班级名称
       COUNT(s.sid) AS student_num -- 学生数(聚合,合法)
FROM class_info c
LEFT JOIN student_score s ON s.class_id = c.class_id
GROUP BY c.class_id, c.class_name;
```

**情况 2:要班级 + 学生明细——别 GROUP BY:**

```sql
SELECT c.class_id,            -- 班级ID
       c.class_name,          -- 班级名称
       s.sid,                 -- 学生ID
       s.student_name,        -- 学生姓名
       s.score                -- 分数
FROM class_info c
LEFT JOIN student_score s ON s.class_id = c.class_id;
```

**情况 3:每个班取一条代表行(如第一个学生)——用窗口函数:**

```sql
SELECT * FROM (
    SELECT c.class_id,        -- 班级ID
           c.class_name,      -- 班级名称
           s.sid,             -- 学生ID
           s.student_name,    -- 学生姓名
           s.score,           -- 分数
           ROW_NUMBER() OVER(PARTITION BY c.class_id ORDER BY s.sid) AS rn
                                -- 按班级分区,按学生ID排序的行号
    FROM class_info c
    LEFT JOIN student_score s ON s.class_id = c.class_id
) t WHERE t.rn = 1;          -- 每班只取行号为1的(第一个学生)
```

旧版 MySQL(5.6 及更早、或关闭严格模式)会随便取一行,是未定义行为;8.0 严格模式直接拒绝,窗口函数是规范写法。

---

## 六、面试一句话总结

- JOIN 本质是嵌套循环，驱动表小、被驱动表 JOIN 列有索引，最快。
- "小表"指 **WHERE 过滤后行数小**，不是表本身小。
- **被驱动表 JOIN 列有索引，比驱动表大小更关键**——这是最常被忽略的点。
- MySQL 8.0.18+ 无索引 JOIN 用 Hash Join，成本 `O(N+M)`，驱动表大小权重下降。
- 优化器自动选驱动表，书写顺序不一定生效；要强制用 `STRAIGHT_JOIN`。
- **功能依赖能省 GROUP BY 列,仅限"GROUP BY 列唯一决定 SELECT 列"——一对多 JOIN 后 `select *` + GROUP BY 必挂 Error 1055。**

---

## 七、实验

对应代码：[experiments/06_join.go](experiments/06_join.go)

```bash
go run ./experiments/ join
```

实验内容：
1. Index NLJ：被驱动表 JOIN 列有二级索引，type=ref（STRAIGHT_JOIN 固定驱动表观察）
2. 无索引场景：8.0.18+ 自动 Hash Join（Extra=Using join buffer (hash join)）
3. 优化器自动调换驱动表：不加 STRAIGHT_JOIN 时反走 class_info 主键（eq_ref）
4. 三表 JOIN 驱动表选择 + STRAIGHT_JOIN 强制顺序对比
