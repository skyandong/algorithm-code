# Elasticsearch 学习笔记

> 目标：理解 ES 核心原理，掌握面试高频考点与生产实践。

## 目录

1. [核心概念与倒排索引](01-核心概念与倒排索引.md) — 倒排索引三件套、FST 压缩、Segment 不可变性、节点角色
2. [分词与 Mapping](02-分词与Mapping.md) — Analyzer 流水线、IK 分词器、text vs keyword、object vs nested、reindex 代价
3. [查询 DSL 与相关性](03-查询DSL与相关性.md) — Query vs Filter Context、bool 查询、match vs term 陷阱、BM25、聚合
4. [分片与写入流程](04-分片与写入流程.md) — 分片路由、NRT 原理、Refresh/Flush/Translog、深分页解决方案
5. [性能优化与生产实践](05-性能优化与生产实践.md) — Bulk API、JVM Heap、冷热分离、ILM、Canal 同步架构

---

## 运行实验

### 启动 ES

```bash
cd notes/elasticsearch

# 启动
docker compose up -d

# 等待约 20 秒，检查是否就绪
curl localhost:9200/_cluster/health?pretty
```

如果 Docker Hub 拉不下来，用官方 registry 手动拉：

```bash
docker pull docker.elastic.co/elasticsearch/elasticsearch:8.13.0
```

### 运行代码

```bash
go run ./experiments/
```

---

## 重点自测

**倒排索引与核心概念**
- [ ] Term Index / Term Dictionary / Posting List 分别存什么？为什么 Term Index 用 FST 且能放内存？
- [ ] Segment 为什么不可变？删除文档后空间什么时候真正释放？
- [ ] ES 查询的分散-收集（Scatter-Gather）流程是什么？

**Mapping 与分词**
- [ ] `text` 和 `keyword` 最本质的区别？什么场景同时需要两者，怎么配置？
- [ ] 索引时和查询时用不同分词器，各自用哪个，为什么？
- [ ] `object` 在什么情况下会查出错误结果？`nested` 怎么解决这个问题？
- [ ] 为什么 Mapping 一旦确定就很难改？改 Mapping 需要经历什么步骤？

**查询与相关性**
- [ ] Query Context 和 Filter Context 最核心的区别？Filter 为什么可以缓存？
- [ ] `match` 和 `term` 查询 text 字段时为什么结果不同？
- [ ] BM25 相比 TF-IDF 有哪两个关键改进？

**分片与写入**
- [ ] 主分片数量为什么创建后不能修改？文档路由公式是什么？
- [ ] 写入 ES 后为什么不能立刻搜索？Refresh 和 Flush 各做什么？
- [ ] 深分页为什么用 `from + size` 会 OOM？`search_after` 如何解决？

**性能与生产**
- [ ] JVM Heap 为什么最多设 32 GB？剩余内存给谁用？
- [ ] 冷热分离架构 Warm 节点一般做哪两个操作？
- [ ] 大批量全量导入时推荐的优化组合？
