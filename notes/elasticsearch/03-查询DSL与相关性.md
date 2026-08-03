# 查询 DSL 与相关性

> **核心认知：ES 查询分两类——Query Context（算相关性分）和 Filter Context（只判断是否匹配）。大多数业务场景是精确过滤 + 全文检索的组合，正确区分两者是性能优化的第一步：Filter 可以缓存，Query 不行。**

---

## Query Context vs Filter Context

| | Query Context | Filter Context |
|--|--------------|----------------|
| 作用 | 计算相关性评分 `_score` | 判断是否满足条件，不算分 |
| 缓存 | 不缓存 | **可缓存**（Bitset 缓存在内存） |
| 性能 | 相对慢 | 相对快 |
| 典型语句 | `match`、`multi_match`、`query_string` | `term`、`terms`、`range`、`exists` |
| 适用场景 | 全文检索、需要按相关度排序 | 精确过滤、范围筛选、枚举匹配 |

**原则：精确匹配/范围查询尽量放 `filter`，只有需要全文检索的字段放 `must`（Query Context）。**

```json
{
  "query": {
    "bool": {
      "must": [
        { "match": { "title": "Elasticsearch 教程" } }
      ],
      "filter": [
        { "term":  { "status": "published" } },
        { "range": { "price": { "gte": 0, "lte": 100 } } }
      ]
    }
  }
}
```

---

## bool 查询四个子句

| 子句 | 含义 | 影响评分 |
|------|------|---------|
| `must` | 必须满足，类似 AND | 是，加分 |
| `should` | 可以满足，类似 OR；单独用时至少满足一个，配合 `must`/`filter` 时可以全不满足 | 是，满足则加分 |
| `must_not` | 必须不满足，类似 NOT | 否（在 Filter Context 执行） |
| `filter` | 必须满足，类似 AND | **否**，可缓存 |

**`should` 的 `minimum_should_match`：**

```json
{
  "bool": {
    "should": [
      { "term": { "tag": "java" } },
      { "term": { "tag": "golang" } },
      { "term": { "tag": "python" } }
    ],
    "minimum_should_match": 2
  }
}
```

---

## 常用查询语句

### 精确查询（Filter Context 专用）

```json
{ "term":   { "status": "published" } }          // 精确匹配单值
{ "terms":  { "tag": ["java", "go"] } }           // 精确匹配多值（类似 IN）
{ "range":  { "age": { "gte": 18, "lte": 30 } } } // 范围查询
{ "exists": { "field": "avatar" } }               // 字段是否存在
```

### 全文查询（Query Context）

```json
// match：对查询词分词后搜索，terms 之间默认 OR
{ "match": { "title": "Elasticsearch 搜索" } }

// match_phrase：短语匹配，词项必须相邻（顺序和位置都要求）
{ "match_phrase": { "title": "Elasticsearch 搜索引擎" } }

// multi_match：跨多个字段搜索
{
  "multi_match": {
    "query": "Elasticsearch",
    "fields": ["title^2", "content"],   // ^2 表示 title 权重是 content 的 2 倍
    "type": "best_fields"               // 取得分最高的字段的分
  }
}
```

### match vs term 的经典陷阱

```
字段类型: text, analyzer: standard
文档: title = "Hello World"
索引后: terms = ["hello", "world"]

term 查询: { "term": { "title": "Hello World" } } → 匹配不到！
  term 不分词，直接查 "Hello World"，但索引里没有这个词项

match 查询: { "match": { "title": "Hello World" } } → 能匹配
  match 先分词成 ["hello", "world"]，再分别查，能找到文档

结论：text 字段用 match；keyword 字段用 term。
```

---

## 相关性评分（BM25）

ES 5.0+ 默认用 BM25 代替 TF-IDF。

### TF-IDF 的问题

- **TF 线性增长**：某个词出现 100 次的文档分数是出现 1 次的 100 倍，不合理
- **没有字段长度归一化**：长文档因包含更多词，词频自然更高，被不公平加分

### BM25 的改进

```
BM25 Score = Σ IDF(t) × TF_saturated(t, d)

TF_saturated = TF / (TF + k1 × (1 - b + b × dl/avgdl))
  k1: 词频饱和系数，默认 1.2（越大饱和越慢）
  b:  字段长度归一化系数，默认 0.75（0 = 不归一化，1 = 完全归一化）
  dl: 当前文档长度
  avgdl: 平均文档长度
```

| 改进点 | 效果 |
|--------|------|
| TF 饱和 | 词频增加对分数的贡献递减，避免高频词碾压 |
| 字段长度归一化 | 短文档中词的权重更高，长文档不占便宜 |
| IDF 与 TF-IDF 相同 | 词越稀有，区分度越高，权重越大 |

### 调整评分的手段

```json
// 1. boost 提升特定字段/词项权重
{ "match": { "title": { "query": "ES", "boost": 2.0 } } }

// 2. function_score：自定义评分函数（如按时间衰减、按销量加权）
{
  "function_score": {
    "query": { "match": { "title": "搜索" } },
    "functions": [
      {
        "gauss": {
          "publish_time": {
            "origin": "now",
            "scale": "7d",
            "decay": 0.5
          }
        }
      }
    ]
  }
}

// 3. rescore：查出 top N 后对这 N 条重新精细评分
```

---

## 聚合（Aggregation）

三类聚合：

| 类型 | 说明 | 类比 SQL |
|------|------|---------|
| Bucket Aggregation | 按条件分桶 | GROUP BY |
| Metric Aggregation | 对桶内数据计算指标 | COUNT / AVG / MAX / SUM |
| Pipeline Aggregation | 对聚合结果再计算 | 对 GROUP BY 结果做二次处理 |

```json
{
  "aggs": {
    "status_group": {
      "terms": { "field": "status" },        // Bucket：按 status 分桶
      "aggs": {
        "avg_price": {
          "avg": { "field": "price" }         // Metric：每桶内计算均价
        }
      }
    }
  }
}
```

**聚合性能注意事项：**
- `terms` 聚合的 `size` 默认 10，增大会消耗更多内存
- 高基数字段（如 user_id）做 `cardinality` 聚合时，使用近似算法（HyperLogLog），有误差
- 聚合要求字段是 `fielddata`（text）或 `doc_values`（keyword/numeric，默认开启）

---

## 自测

- [ ] Query Context 和 Filter Context 最核心的区别是什么？什么情况下用 filter？
- [ ] `must`、`should`、`filter`、`must_not` 各自什么语义？哪些影响评分？
- [ ] `match` 和 `term` 查询 text 类型字段时，结果为什么可能不同？
- [ ] BM25 相比 TF-IDF 有哪两个关键改进？为什么 TF 饱和是合理的？
- [ ] 想让新文章比旧文章排名靠前，怎么在 ES 里实现？
