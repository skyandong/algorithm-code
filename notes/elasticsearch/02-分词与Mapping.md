# 分词与 Mapping

> **核心认知：全文检索的质量取决于两件事——索引时把文本切成什么词（分词器），字段用什么类型存（Mapping）。这两个决策在创建索引时就固定了，之后改很贵（需要 reindex），所以必须提前想清楚。**

---

## 分词器（Analyzer）

### 三阶段流水线

```
原始文本
  ↓ Character Filter（预处理）   去 HTML 标签、替换字符（& → and）
  ↓ Tokenizer（分词）             按规则切成 Token
  ↓ Token Filter（后处理）        转小写、去停用词（the/a/is）、词干提取、同义词扩展
分词结果（Terms）
```

### 常用分词器对比

| 分词器 | 切分规则 | 典型场景 |
|--------|---------|---------|
| standard | 按空格和标点切分，转小写 | 英文默认 |
| whitespace | 只按空格切分，不做其他处理 | 日志字段 |
| keyword | 不分词，整个字段作为一个词项 | 精确匹配字段（配合 filter 用） |
| ik_max_word | 中文细粒度，尽量多拆词 | 中文索引 |
| ik_smart | 中文智能，尽量少拆词 | 中文查询 |

**ik_max_word vs ik_smart 的最佳实践：**
- 索引时用 `ik_max_word`：多拆词，提高召回率（不遗漏）
- 查询时用 `ik_smart`：智能匹配，提高准确率（不引入噪音）

```json
{
  "mappings": {
    "properties": {
      "title": {
        "type": "text",
        "analyzer": "ik_max_word",
        "search_analyzer": "ik_smart"
      }
    }
  }
}
```

---

## Mapping 与字段类型

### Dynamic Mapping 的陷阱

ES 可以自动推断字段类型（Dynamic Mapping），但容易出错：

| 陷阱 | 原因 |
|------|------|
| 数字被推断为 `long`，其实想要 `keyword` | 如订单号 "123456"，想精确匹配但被分词/映射为数值 |
| 日期字符串被推断为 `date`，格式稍有不同就报错 | |
| 首次写入的字段类型确定后，后续不能修改 | Mapping 变更只能 reindex，代价极高 |

**生产建议：关闭 Dynamic Mapping，手动定义。**

```json
{
  "mappings": {
    "dynamic": "strict"
  }
}
```

### 核心字段类型

| 类型 | 特点 | 适用场景 |
|------|------|---------|
| `text` | 会被分词，支持全文检索，**不能**聚合/排序 | 文章标题、内容 |
| `keyword` | 不分词，精确匹配，支持聚合/排序/过滤 | 状态、标签、枚举值 |
| `long` / `integer` / `float` | 数值类型，支持范围查询 | 价格、年龄、数量 |
| `date` | 日期，支持 format 自定义，内部存毫秒时间戳 | 时间字段 |
| `boolean` | true / false | 开关字段 |
| `object` | 嵌套 JSON 对象，内部字段被扁平化索引 | 嵌套结构 |
| `nested` | 嵌套对象数组，保持对象独立性，查询需用 nested query | 评论列表、标签列表 |

### text vs keyword 的核心区别

```
text: "iPhone 15 Pro Max"
  → 分词后: ["iphone", "15", "pro", "max"]
  → 可以搜索 "pro"，可以全文检索
  → 不能 GROUP BY、不能精确等值过滤

keyword: "iPhone 15 Pro Max"
  → 存储为整体，不分词
  → 只能精确匹配 "iPhone 15 Pro Max"（大小写敏感）
  → 可以聚合、排序、精确过滤
```

**既要搜索又要聚合：用 fields 多字段特性**

```json
{
  "title": {
    "type": "text",
    "analyzer": "ik_max_word",
    "fields": {
      "keyword": {
        "type": "keyword",
        "ignore_above": 256
      }
    }
  }
}
```

- `title`：全文检索
- `title.keyword`：精确匹配、聚合、排序

### object vs nested

```
object 扁平化索引的问题：
  文档：
    comments: [
      { author: "Alice", like: true },
      { author: "Bob",   like: false }
    ]
  扁平化后：
    comments.author: ["Alice", "Bob"]
    comments.like:   [true, false]

  查询"Alice 且 like=true"：
    ES 会找到这条文档（因为 Alice 确实在 author 里，true 确实在 like 里）
    但实际上 Alice 对应的是 like=false！

nested 保持对象独立：
  每个 comment 是独立的 Lucene 文档
  nested query 只在单个 comment 内做条件组合
  → 结果正确
```

代价：nested 对象会生成额外的 Lucene 文档，查询需要 `nested` 查询语法，性能更重。

---

## Reindex 的代价

Mapping 一旦确定，字段类型不能原地修改（除了少数加字段的操作）。要改类型必须：

1. 创建新索引（新 Mapping）
2. 把旧索引数据全量迁移到新索引（`_reindex` API）
3. 切换别名（alias）

这个过程对大索引可能需要数小时，期间新旧索引可能双写。**所以 Mapping 设计是 ES 最重要的前期决策。**

---

## 自测

- [ ] Dynamic Mapping 有哪些常见坑？生产为什么要关闭？
- [ ] `text` 和 `keyword` 最本质的区别是什么？什么场景同时需要两者？
- [ ] 一个字段既要全文检索又要聚合，怎么配置 Mapping？
- [ ] `object` 和 `nested` 的区别是什么？`object` 在什么情况下会查出错误结果？
- [ ] 为什么说 Mapping 设计要慎重？改 Mapping 的代价是什么？
- [ ] 索引时和查询时用不同分词器，各自用什么分词器，为什么？
