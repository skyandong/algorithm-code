# PromQL 实战：两种向量，一套组合拳

> **核心认知：** PromQL 只有一个真正需要跨过去的心智门槛——**瞬时向量（instant vector）和区间向量（range vector）是两种东西**。瞬时向量是"此刻每一条 series 各有一个值"（图表上的一个点集）；区间向量是"过去一段时间里每一条 series 各有一串值"（只有它能喂给 `rate`/`increase` 这类**随时间变化**的函数）。绝大多数 PromQL 报错（"expected type range vector"）和绝大多数错误结果，都是把这两种向量弄混了。跨过去之后，剩下的就是十几个函数的排列组合。

前置知识：[07](07-指标体系与数据模型.md)（series、Counter、Histogram 桶）、[08](08-Prometheus架构与抓取.md)（scrape_interval 与窗口的关系）。

---

## 目录

1. [两种向量：PromQL 的地基](#1-两种向量promql-的地基)
2. [rate、irate、increase：Counter 三兄弟](#2-rateirateincreasecounter-三兄弟)
3. [Counter 重置与外推：rate 帮我们兜了什么](#3-counter-重置与外推rate-帮我们兜了什么)
4. [聚合：by 与 without 的取舍](#4-聚合by-与-without-的取舍)
5. [histogram_quantile：算分位数](#5-histogram_quantile算分位数)
6. [Recording Rules：预计算的唯一理由](#6-recording-rules预计算的唯一理由)
7. [四个黄金信号落地成 PromQL](#7-四个黄金信号落地成-promql)
8. [面试高频](#8-面试高频)

---

## 1. 两种向量：PromQL 的地基

| | 瞬时向量 instant vector | 区间向量 range vector |
|---|---|---|
| 形态 | 每条 series 一个值 | 每条 series 一串（时间, 值） |
| 写法 | `http_requests_total` | `http_requests_total[5m]` |
| 能画成图 | 能（就是图） | **不能**，只能喂给函数 |
| 典型消费 | 直接展示、比较、算术 | `rate()` / `increase()` / `avg_over_time()` 等 `*_over_time` 族 |

**为什么必须区分**：`http_requests_total` 这个值**本身没有意义**（它是从服务启动累计到现在的总数，会一直涨）。要得到有意义的量，必须"看它怎么变"——而**变化需要一段时间窗口**，所以要先 `[5m]` 变成区间向量，再交给 `rate()` 变成速率。**这个两步动作是 PromQL 里最高频的套路**。

**选择器的写法**（跟 label 匹配的三种）：

```promql
http_requests_total{status="500"}              # 相等
http_requests_total{status!="200"}             # 不等
http_requests_total{status=~"5.."}             # 正则（完全匹配）
http_requests_total{route=~"/api/.*",env="prod"}  # 多条件 AND
```

**偏移量**：`http_requests_total offset 1d`（一天前的值），`rate(x[5m] offset 1d)`（一天前那个时刻的 5 分钟速率）——**同比环比全靠它**。

---

## 2. rate、irate、increase：Counter 三兄弟

| 函数 | 返回 | 语义 | 用在 |
|---|---|---|---|
| `rate(x[5m])` | 每秒速率 | 窗口内**平均**速率（做了外推与重置修正） | **画图的默认选择**（告警、面板） |
| `irate(x[5m])` | 每秒速率 | 只看**最后两个样本**算瞬时速率 | 看毛刺；**不要用来告警** |
| `increase(x[5m])` | 增量 | 窗口内增加了多少（`rate × 窗口秒数`） | "过去 5 分钟来了多少请求" |

**三者的关系**：`increase(x[5m]) == rate(x[5m]) * 300`。没有本质区别，只是单位。

**`rate` vs `irate` 的选择（高频题）**：

```text
rate:  窗口内所有样本参与, 做最小二乘拟合 → 平滑, 抗抖动, 但会"抹平"毛刺
irate: 只取最后两个样本 → 反应极快, 能看出尖刺, 但图会锯齿状抖动
```

- **画面板/配告警 → 一律 `rate`**：你要的是趋势，不是噪声。`irate` 的锯齿会让告警在阈值附近反复翻转。
- **看毛刺 → `irate`**：`rate` 的平滑会把 1 秒的尖峰稀释掉，你怀疑有瞬时抖动时用 `irate` 看一眼。

**窗口取多大（经验法则）**：`[5m]` 是默认值，但**必须 ≥ 4 × scrape_interval**。15s 抓取配 `[1m]` 也行，`[5m]` 更稳；**绝不能 `[10s]` 配 15s 抓取**——窗口里可能只有 0~1 个样本，rate 直接算不出来或失真严重。窗口越大越平滑但响应越慢（告警延迟 = 半个窗口量级）。

---

## 3. Counter 重置与外推：rate 帮我们兜了什么

这是"为什么不能自己写 `(now - before) / 300`"的答案。`rate()` 在算增量时处理了两件事：

**① Counter 重置（进程重启归零）**：

```text
样本序列: 1000 → 1010 → 1015 → 3 → 8 → 12     （进程重启, 计数归零）
朴素算法: (12 - 1000) / 时间 = 巨大的负数      ✗ 完全错误
rate:     检测到后值 < 前值 → 判定重置 → 把重置后的值当作 "前值 + 现值" 处理
          (1015 - 1000) + (12 - 0) = 27       ✓
```

所以**你永远不需要在应用里处理重启归零**——这是 Counter 语义 + `rate` 的分工。

**② 边界外推**：窗口边界通常不正好落在样本点上，`rate` 会把窗口两端的部分区间按比例外推，让结果连续（否则每次抓取偏移都会让值跳变）。代价是**结果可能是小数**（即使计数是整数），这完全正常。

**③ 抓取失败的容忍**：窗口内丢了一两个抓取点，`rate` 仍能算（用剩余点拟合），只是精度下降。这也是为什么短暂的监控抖动不该直接触发业务告警——**告警规则里通常会加 `for: 5m`**（见 10 篇）。

---

## 4. 聚合：by 与 without 的取舍

```promql
sum(rate(http_requests_total[5m])) by (route)        # 按 route 分组求和, 丢掉其他标签
sum(rate(http_requests_total[5m])) without (instance) # 保留除 instance 外的所有标签
```

**WHY 两种写法**：`by` 是白名单（我知道要留什么），`without` 是黑名单（我知道要丢什么）。**下钻排查时常用 `without (instance)`**——你想看服务整体，但不想枚举所有其他标签（method/status/route 全都要留着做对比）。

**聚合函数族**：`sum`（总量）、`avg`（平均，注意 instance 间平均 vs 加权平均的语义）、`min`/`max`（找最差的实例）、`count`（多少个实例）、`topk(5, ...)`（TopN——**找最慢的 5 个接口**）、`quantile(0.9, ...)`（**按 series 算分位，不是按样本**，与 `histogram_quantile` 完全不同，别搞混）。

**必须先 rate 再 sum，不能反过来**：

```promql
sum(rate(http_requests_total[5m]))         ✓ 先算每条 series 的速率, 再相加
rate(sum(http_requests_total)[5m])         ✗ 类型错误: sum 返回瞬时向量, 不能加 [5m]
```

这是新手最常见的写法错误——**区间选择器只能贴在原始选择器后面**。

---

## 5. histogram_quantile：算分位数

```promql
histogram_quantile(0.99,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le)
)
```

**两个必须做对的动作**：

1. **`by (le)` 必须保留 `le` 标签**——分位数计算全靠它。如果你写成 `by (route)`，桶信息丢了，函数无从下手，结果毫无意义。
2. **必须先按 `le` 之外的维度聚合**：多实例下各实例的桶要相加（07 篇 §3 讲过桶可加），`sum by (le)` 正是做这件事。如果还要按路由拆，就 `sum by (le, route)`。

**精度取决于桶宽**：`histogram_quantile` 做的是桶内线性插值（07 篇 §3 手算过）——**桶宽 0.5s，P99 的最大误差就是 0.5s**。所以桶边界必须贴 SLO 设。一个常见错误：桶设成 `0.1, 1, 10, 60`，然后抱怨 P99 不准——**不是函数不准，是桶太粗**。

**分桶建议**：指数增长（`0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10`），在 SLO 附近加密（SLO 200ms 就在 0.1~0.3 之间多切几刀）。

**平均延迟怎么算**（不用分位数时）：

```promql
rate(http_request_duration_seconds_sum[5m]) / rate(http_request_duration_seconds_count[5m])
```

分子是"每秒消耗的总秒数"，分母是"每秒请求数"，相除即平均耗时。

---

## 6. Recording Rules：预计算的唯一理由

```yaml
groups:
  - name: pay-service
    rules:
      - record: job:http_requests:rate5m
        expr: sum(rate(http_requests_total[5m])) by (job)
```

Recording rule = **把一条昂贵查询的结果定期算好，存成一条新的 series**，之后所有面板和告警都查这条新 series。

**唯一的理由是性能**：一个被 20 个面板引用的、扫百万 series 的聚合查询，每次开面板都重算一遍会拖垮 Prometheus。预计算后查询退化成读一条 series。

**命名约定**：`level:metric:operation`（如 `job:http_requests:rate5m`）——冒号分隔让来源一目了然。

**不要过度使用**：只有"高频访问 + 计算昂贵"的查询才值得预计算。**加了 recording rule 就多了一份要维护的配置，且结果有一轮求值间隔的延迟**。

---

## 7. 四个黄金信号落地成 PromQL

把 [05 篇](05-可观测性.md) 的四个黄金信号翻译成可执行的查询（**这就是你的第一块看板**）：

```promql
# ① 流量 (Traffic): 每秒请求数, 按路由拆
sum(rate(http_requests_total[5m])) by (route)

# ② 错误 (Errors): 错误率, 0~1
sum(rate(http_requests_total{status=~"5.."}[5m]))
/ sum(rate(http_requests_total[5m]))

# ③ 延迟 (Latency): P99, 注意 by (le)
histogram_quantile(0.99,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le, route))

# ④ 饱和度 (Saturation): 连接池使用率 / 队列深度 / CPU
go_sql_connections_in_use / go_sql_connections_max
rate(node_cpu_seconds_total{mode="idle"}[5m])   # 反过来用: 1 - idle 即使用率
```

**分母为 0 的坑**：错误率那条，如果服务流量为 0，分母是 0 → 结果 `NaN` 或无数据。**告警表达式里要处理**：加 `and sum(rate(http_requests_total[5m])) > 1`（有量才判），否则凌晨没流量时会误报或漏报。

**RED 看板**（服务视角，RED = 上面前三条）：Rate / Errors / Duration 三行图，是绝大多数服务的标准看板布局（见 11 篇）。

---

## 8. 面试高频

**Q1：瞬时向量和区间向量的区别？**
瞬时向量是每条 series 一个值（能直接画图），区间向量是每条 series 一串带时间戳的值（只能喂给 `rate`/`increase`/`*_over_time` 这类函数）。Counter 原始值无意义，要先 `[5m]` 转区间向量再 `rate()` 得速率——这是 PromQL 最高频的套路。

**Q2：rate 和 irate 的区别？怎么选？**
`rate` 用窗口内所有样本做拟合，平滑抗抖动，是画图/告警的默认；`irate` 只取最后两个样本，反应快能看毛刺但图会锯齿、会让阈值附近的告警反复翻转。**告警和面板一律 rate，怀疑有瞬时抖动时用 irate 看一眼。**

**Q3：rate 的窗口怎么取？**
必须 ≥ 4 × scrape_interval（15s 抓取至少 `[1m]`，常用 `[5m]`）。窗口越大越平滑但响应越慢。绝不能 `[10s]` 配 15s 抓取——窗口内样本不足会算不出或严重失真。

**Q4：服务重启了 Counter 归零，QPS 会不会算成负数？**
不会。`rate` 检测到后值小于前值时判定为重置，把重置后段当作从 0 开始单独计算增量再相加。**所以应用侧永远不需要处理重启归零**——这正是 Counter 语义 + rate 的分工。

**Q5：多实例下怎么算全局 P99？**
`histogram_quantile(0.99, sum(rate(..._bucket[5m])) by (le))`。两个要点：`by (le)` 必须保留 le 标签；必须先把各实例的同名桶 `sum` 起来（桶可加，分位数不可加）。精度取决于桶宽——桶太粗不是函数不准。

**Q6：为什么 `rate(sum(x))` 是错的？**
区间选择器只能贴在原始选择器后面。`sum()` 返回的是瞬时向量，不能再加 `[5m]`。正确写法是先 `rate` 再 `sum`：`sum(rate(x[5m]))`。

**Q7：错误率告警在没流量时怎么处理？**
分母为 0 会得到 NaN/无数据，导致误报或漏报。告警表达式要加流量守卫：`... and sum(rate(http_requests_total[5m])) > 1`。

**Q8：recording rule 什么时候用？**
唯一理由是性能——高频访问且计算昂贵的聚合查询，预计算成新 series 供面板和告警复用。命名 `level:metric:operation`。不要过度使用，多一份配置且有一轮求值延迟。

---

本篇对应实验：experiments/09_promql.go（手写 rate/increase/irate 与 counter 重置修正：对比朴素算法与修正算法在重启场景下的差异，验证窗口不足导致的失真）、experiments/08_histogram.go（手写分桶与 `histogram_quantile` 线性插值，并演示粗桶带来的误差量级）
