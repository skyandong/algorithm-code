# WHERE 与 HAVING

> **核心认知：WHERE 过滤"行"(聚合前),HAVING 过滤"组"(聚合后)。执行顺序 FROM → WHERE → GROUP BY → HAVING → SELECT → ORDER BY → LIMIT,所以 WHERE 里不能用聚合字段、HAVING 里能。**

---

## 一、本质区别

| 维度 | WHERE | HAVING |
|---|---|---|
| 作用时机 | 聚合**前**过滤原始行 | 聚合**后**过滤分组 |
| 能用聚合函数吗 | ✗ 不能(SUM/AVG/COUNT 还没算) | ✓ 能 |
| 能用 SELECT 别名吗 | ✗ 不能(SELECT 还没执行) | ✓ 能(MySQL 特例,标准 SQL 不行) |
| 能用 GROUP BY 列吗 | ✓ 能 | ✓ 能 |
| 配合 | 不需要 GROUP BY 也能用 | 通常配合 GROUP BY |

---

## 二、执行顺序(关键)

SQL 书写顺序 ≠ 执行顺序。理解了执行顺序,WHERE/HAVING 的边界就自然清楚了。

```
书写顺序:  SELECT → FROM → WHERE → GROUP BY → HAVING → ORDER BY → LIMIT
执行顺序:  FROM → WHERE → GROUP BY → HAVING → SELECT → ORDER BY → LIMIT
           ①      ②       ③          ④        ⑤       ⑥         ⑦
```

- WHERE 在 ②,GROUP BY 在 ③——WHERE 先执行,此时还没聚合,所以 WHERE 用不了 `AVG()`、`COUNT()`。
- HAVING 在 ④,聚合已算完——所以 HAVING 能用 `AVG(s.score) > 70`。
- SELECT 在 ⑤,在 HAVING 之后——所以 HAVING 用 SELECT 别名是 MySQL 的"开绿灯",标准 SQL 其实不允许。

---

## 三、反面案例:WHERE 用聚合字段(必挂)

```sql
SELECT
    g.grade_name,                          -- 年级名称
    c.class_name,                          -- 班级名称
    COUNT(*) AS student_num,               -- 学生数(聚合)
    ROUND(AVG(s.score),2) avg_score        -- 平均分(聚合)
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
WHERE avg_score >= 70                      -- ✗ 错!WHERE 不能用聚合字段别名
GROUP BY g.grade_name, c.class_name;
```

**真实报错:** `Error 1054 (42S22): Unknown column 'avg_score' in 'where clause'`

**原因:** WHERE 在 GROUP BY 之前执行,此时 `avg_score`(AVG 的别名)还没算出来,MySQL 不认识它。

---

## 四、正面写法:聚合过滤用 HAVING

```sql
SELECT
    g.grade_name,                          -- 年级名称
    c.class_name,                          -- 班级名称
    COUNT(*) AS student_num,               -- 学生数(聚合)
    ROUND(AVG(s.score),2) avg_score        -- 平均分(聚合)
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name
HAVING avg_score >= 70;                     -- ✓ HAVING 过滤聚合结果(MySQL 允许用 SELECT 别名)
```

---

## 五、WHERE 和 HAVING 同时用

WHERE 过滤原始行(减少进入聚合的数据量),HAVING 过滤聚合结果。**能放 WHERE 的条件不要放 HAVING**,因为 WHERE 先过滤能让聚合算得更少。

```sql
SELECT
    c.class_name,                          -- 班级名称
    COUNT(*) AS student_num,               -- 学生数(聚合)
    ROUND(AVG(s.score),2) avg_score        -- 平均分(聚合)
FROM class_info c
LEFT JOIN student_score s ON c.class_id = s.class_id
WHERE s.score >= 60                        -- ✓ WHERE 先过滤:只算及格的人(减少聚合输入)
GROUP BY c.class_id, c.class_name
HAVING avg_score >= 75;                    -- ✓ HAVING 再过滤:班级平均 >= 75(过滤聚合结果)
```

**反例(把能放 WHERE 的条件塞进 HAVING):**

```sql
-- 不好:score >= 60 不是聚合条件,放 HAVING 里也能跑,但所有行都进了聚合,白白多算
SELECT
    c.class_name,                          -- 班级名称
    COUNT(*) AS student_num,
    ROUND(AVG(s.score),2) avg_score
FROM class_info c
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY c.class_id, c.class_name
HAVING avg_score >= 75 AND s.score >= 60;  -- 不推荐:score >= 60 是行级过滤,应该放 WHERE
```

**原则:** 行级条件(不涉及聚合)放 WHERE,组级条件(涉及聚合)放 HAVING。

---

## 六、HAVING 用 SELECT 别名——MySQL 特例

```sql
SELECT
    c.class_name,                          -- 班级名称
    ROUND(AVG(s.score),2) avg_score        -- 别名 avg_score
FROM class_info c
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY c.class_id, c.class_name
HAVING avg_score >= 75;                    -- ✓ MySQL 允许直接用别名
```

标准 SQL 要求 HAVING 写成 `HAVING AVG(s.score) >= 75`(原始聚合表达式),MySQL 扩展了允许用 SELECT 别名。**面试时提一句"MySQL 允许,标准 SQL 不允许"是加分项。**

---

## 七、速查表

| 场景 | 用 WHERE | 用 HAVING |
|---|---|---|
| `score > 60`(行级过滤) | ✓ | ✗(能跑但不推荐) |
| `AVG(score) > 70`(聚合过滤) | ✗(报错) | ✓ |
| `student_name = '小明'`(行级) | ✓ | ✓(不推荐) |
| `COUNT(*) > 3`(聚合过滤) | ✗(报错) | ✓ |
| 减少聚合输入量 | ✓ | ✗ |

---

## 八、别名(SELECT 别名)能用在哪——实测

> SELECT 里定义的别名(如 `ROUND(AVG(s.score),2) class_avg`),在不同子句里能不能直接用?以下是 MySQL 8 实测结果。

```sql
SELECT
    g.grade_name,                          -- 年级名称
    c.class_name,                          -- 班级名称
    ROUND(AVG(s.score),2) class_avg        -- 别名 class_avg 在这里定义
FROM grade g
JOIN class_info c ON g.grade_id = c.grade_id
LEFT JOIN student_score s ON c.class_id = s.class_id
GROUP BY g.grade_id, g.grade_name, c.class_id, c.class_name;
```

| 位置 | 能用别名 `class_avg`? | 实测 |
|---|---|---|
| `WHERE class_avg >= 70` | ✗ 不能 | `Error 1054: Unknown column 'class_avg' in 'where clause'` |
| `GROUP BY class_avg` | ✗ 不能 | 别名在 GROUP BY 之后才生成 |
| `HAVING class_avg >= 70` | ✓ 能 | MySQL 特例,标准 SQL 不允许 |
| 普通 `ORDER BY class_avg` | ✓ 能 | ORDER BY 在 SELECT 之后执行 |
| 窗口函数 `ORDER BY class_avg` | ✗ **不能** | `Error 1054: Unknown column 'class_avg' in 'window order by'` |
| 窗口函数 `PARTITION BY class_avg` | ✗ 不能 | `Error 1054: Unknown column 'class_avg' in 'window partition by'` |

### 为什么窗口函数里不能用,普通 ORDER BY 能?

执行顺序(简化):

```
FROM → WHERE → GROUP BY → (窗口函数) → SELECT → ORDER BY → LIMIT
                              ↑              ↑
                       窗口函数在这算    别名在这之后才生成
```

- **普通 ORDER BY** 在 SELECT 之后,别名已生成,能用。
- **HAVING** 在 SELECT 之后(MySQL 开绿灯,标准 SQL 不允许)。
- **窗口函数** 在 SELECT **之前**算,此时别名还没被解析出来,所以窗口函数的 `ORDER BY` / `PARTITION BY` 只能用**原始表达式** `AVG(s.score)`,不能用别名。

### 反直觉点(面试常被坑)

```sql
-- 普通 ORDER BY:能用别名 ✓
SELECT ..., ROUND(AVG(s.score),2) class_avg
FROM ...
GROUP BY ...
ORDER BY class_avg DESC;                   -- ✓ 跑通

-- 窗口函数里的 ORDER BY:不能用别名 ✗
SELECT ..., ROUND(AVG(s.score),2) class_avg,
       RANK() OVER(PARTITION BY g.grade_id ORDER BY class_avg DESC) rn
--                                                 ↑
--                                          ✗ 报错,只能写 AVG(s.score)
FROM ...
GROUP BY ...;
```

**同一个 ORDER BY,在普通位置能用别名、在窗口函数里不能用**——这是 MySQL 执行顺序决定的反直觉点。

### 速记

- 别名能用:普通 `ORDER BY`、`HAVING`(MySQL 特例)。
- 别名不能用:`WHERE`、`GROUP BY`、**窗口函数的 `PARTITION BY` 和 `ORDER BY`**。
- 窗口函数里只能用原始表达式。

---

## 九、面试一句话总结

- WHERE 过滤行(聚合前),HAVING 过滤组(聚合后)——执行顺序决定边界。
- WHERE 用聚合字段报 Error 1054(Unknown column),HAVING 能用。
- 能放 WHERE 的条件别放 HAVING——WHERE 先过滤,减少聚合输入。
- HAVING 用 SELECT 别名是 MySQL 特例,标准 SQL 不允许(面试加分点)。
- **别名能用位置:普通 ORDER BY、HAVING;不能用:WHERE、GROUP BY、窗口函数的 PARTITION BY/ORDER BY**——窗口函数里只能用原始表达式。
- 一句话记忆:**"行级 WHERE,组级 HAVING"**。
