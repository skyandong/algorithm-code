# 告警与 Alertmanager：告警是给人看的

> **核心认知：** 告警系统的目标**不是"发现所有异常"，而是"保证每一条被发出的告警都值得有人立刻放下手头的事"**。一条没人看的告警比没有告警更糟——它训练值班人员对告警脱敏，等真事故来的时候同样被忽略。所以告警设计的核心动作是**减法**：基于症状而非原因告警、加 `for` 抗抖、用 Alertmanager 把一次故障收敛成一条通知。**判断一套告警好坏的标准是"过去三个月里，有多少条告警是真的需要人动的"**——低于 30% 就是在制造噪声。

前置知识：[09](09-PromQL实战.md)（告警表达式就是 PromQL）、[05](05-可观测性.md)（四个黄金信号、饱和度是先行指标）。

---

## 目录

1. [基于症状 vs 基于原因](#1-基于症状-vs-基于原因)
2. [告警规则语法与 for 的作用](#2-告警规则语法与-for-的作用)
3. [Alertmanager 四件事：分组、抑制、静默、路由](#3-alertmanager-四件事分组抑制静默路由)
4. [告警分级与值班](#4-告警分级与值班)
5. [四条通用告警模板](#5-四条通用告警模板)
6. [噪声治理：把告警数量降一个数量级](#6-噪声治理把告警数量降一个数量级)
7. [面试高频](#7-面试高频)

---

## 1. 基于症状 vs 基于原因

| | 基于原因（cause-based） | 基于症状（symptom-based） |
|---|---|---|
| 表达式 | `CPU > 80%`、`磁盘剩余 < 10%` | `错误率 > 1%`、`P99 > 500ms` |
| 本质 | 某个资源指标越界 | **用户正在受影响** |
| 问题 | 大量原因不导致症状（CPU 高但一切正常 → 噪声） | 需要额外信息才能定位 |

**结论：面向值班的告警（Page）必须基于症状，基于原因的只做记录（Ticket）或看板。**

```text
CPU 90% 但服务 P99 正常、错误率正常 → 不打电话, 记一张工单白天看
错误率 3%                          → 立刻打电话, 不管 CPU 是多少
```

**唯一的例外是"即将耗尽"类**：磁盘还剩 6 小时满、证书 7 天后过期——这类是**预测性告警**，用户还没受影响但再不动就出事，属于正当的基于原因告警。

**WHY 这个原则如此重要**：微服务的故障传播是网状的，**一个根因可能表现为几十个"原因指标异常"**（下游全挂 → 上游十几个服务的 CPU、连接池、队列全异常）。如果按原因告警，一次故障会打出几十条通知——值班的人在噪声里找根因的时间，比按症状告警再顺着依赖图下钻要长得多。

---

## 2. 告警规则语法与 for 的作用

```yaml
groups:
  - name: pay-service
    rules:
      - alert: HighErrorRate
        expr: |
          sum(rate(http_requests_total{status=~"5.."}[5m]))
            / sum(rate(http_requests_total[5m])) > 0.01
          and sum(rate(http_requests_total[5m])) > 1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.job }} 错误率 {{ $value | humanizePercentage }}"
          runbook_url: https://wiki/xxx
```

**三个字段的职责**：

- `expr`：一条返回**非空即触发**的 PromQL。返回空（无数据）则不触发——这就是为什么 09 篇强调要加流量守卫。
- **`for`：必须持续满足多久才真正触发（核心字段）**。这是抗抖动的关键：一次网络抖动、一次 GC 停顿、一次发版重启都会让指标瞬间越界，`for: 5m` 让这类瞬时噪声在 `Pending` 阶段自行消解。**`for` 的取值 ≈ 你愿意接受的告警延迟**，核心服务 2~5m，非核心可 15m。
- `labels` / `annotations`：`severity` 决定路由到谁；`summary` 是给人看的一句话（**必须自带足够信息，让人不用打开 Grafana 就知道大概发生了什么**）；`runbook_url` 是排障手册——**没有 runbook 的告警不是告警，是谜题**。

**告警状态机**（理解它能解释很多现象）：

```text
Inactive → (expr 满足) → Pending → (持续满足 for) → Firing → (expr 不满足) → Inactive(发 resolved)
                             ↑                                      ↓
                        (不满足则回退)                    Alertmanager 按 group_wait 发出通知
```

**`keep_firing_for`**（进阶）：让告警在条件消失后仍保持 Firing 一小段时间，用于避免"抖动—恢复—抖动"造成的重复通知。

---

## 3. Alertmanager 四件事：分组、抑制、静默、路由

Prometheus 只负责把 Firing 的告警推给 Alertmanager，剩下的**全部运营逻辑**在这里（08 篇讲过为什么要独立）：

**① 分组（group_by）**：把同一次故障产生的几十条告警**合并成一条通知**。

```yaml
route:
  group_by: [alertname, job]
  group_wait: 30s      # 收到第一条后等 30s, 攒一攒同组的其他告警
  group_interval: 5m   # 同组后续通知的间隔
  repeat_interval: 4h  # 同一个告警多久重复发一次（防疲劳）
```

**WHY 要 `group_wait`**：一次机房故障会在 2 秒内打出 50 条告警。等 30 秒让它们攒成一条"50 条告警"的通知，而不是连发 50 条独立消息——**这是把"通知数量"从"故障规模"解耦出来的关键**。

**② 抑制（inhibit_rules）**：**高等级告警发生时，静默它已知会引发的低等级告警**。

```yaml
inhibit_rules:
  - source_match: {severity: critical}
    target_match: {severity: warning}
    equal: [job]      # 同一个 job 内才抑制
```

典型场景：整个服务挂了（critical）→ 它的延迟高、队列深、CPU 高（warning）全被抑制。**值班的人只需要看到"服务挂了"这一条**。

**③ 静默（silence）**：**人工**按 label 匹配临时屏蔽告警（发版窗口、已知问题修复中）。与抑制的区别：抑制是配置里的长期规则，静默是运维临时打的（有过期时间）。

**④ 路由（route）**：树形匹配，决定告警发给谁。

```yaml
route:
  receiver: default
  routes:
    - matchers: [severity="critical"]
      receiver: pager          # 电话/短信叫醒
    - matchers: [severity="warning"]
      receiver: wechat-group   # 只发群
```

**去重**：Alertmanager 会对完全相同的告警（同 label 集）自动去重，这是它最基础的功能。

---

## 4. 告警分级与值班

| 级别 | 语义 | 通知渠道 | 响应要求 |
|---|---|---|---|
| **critical / page** | 用户正在受影响，或即将受影响 | 电话/短信/值班 App | **立即**（5~15 分钟） |
| **warning / ticket** | 异常但当前无害 | 群里发消息 | 当班处理（小时级） |
| **info** | 信息类（发版完成、扩缩容） | 只进看板/日志 | 不要求响应 |

**分级的原则**：**能不能等到明天早上**——能等的就不是 page。PM 凌晨三点被叫起来处理一个"磁盘 70%"的告警，是告警系统设计失败的典型症状。

**每条 page 级告警必须配的三样东西**：①一句话 summary（发生了什么）；②runbook 链接（怎么办）；③影响面（多少用户、哪个功能）。**没有这三样的告警不该进 page 通道。**

---

## 5. 四条通用告警模板

绝大多数服务需要的告警不超过这几条（**从"通用"开始，按需加"业务"**）：

```promql
# ① 服务不可用（最基础, 来自 pull 模式的免费午餐）
up{job="pay-service"} == 0            # for: 1m, critical

# ② 错误率
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) > 0.01
  and sum(rate(http_requests_total[5m])) > 1     # for: 5m, critical

# ③ 延迟（症状型; 从 SLO 反推阈值, 不是拍脑袋）
histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le)) > 0.5
                                                  # for: 10m, warning → 超 2 倍才 critical

# ④ 饱和度（先行指标, 05 篇强调过它最有价值）
go_sql_connections_in_use / go_sql_connections_max > 0.8   # for: 15m, warning
predict_linear(node_filesystem_free_bytes[6h], 4*3600) < 0  # 4 小时后磁盘满, critical
```

**阈值从哪来**：延迟阈值**从 SLO 反推**（SLO 是 200ms，告警就设在 2~4 倍 SLO），不是"感觉 500ms 比较慢"。错误率阈值看业务容忍度（支付 0.1% 就很严重，内容推荐 1% 可接受）。

**`predict_linear` 是预测性告警的利器**：用最近 6 小时的趋势线性外推，判断 4 小时后会不会磁盘满——**比"剩余 10% 告警"聪明得多**，因为 10% 对 1TB 盘和对 100GB 盘含义完全不同。

---

## 6. 噪声治理：把告警数量降一个数量级

按收益从高到低：

1. **删掉所有基于原因的 page 告警**（CPU/内存/GC 次数 → 降级为看板或 ticket）。这一条通常能砍掉一半。
2. **加 `for`**：瞬时抖动型告警全部消解（注意 `for` 太长会延迟真实告警，核心服务 2~5m 起步）。
3. **配 `group_by` + `group_wait`**：50 条变 1 条。
4. **配抑制规则**：critical 抑制同 job 的 warning。
5. **加流量守卫**：`and sum(rate(...)) > 1`，消掉凌晨低流量时的比例类误报。
6. **定期复盘**：每季度拉一次"过去三个月所有 page 告警"，逐条问"这条真的需要叫醒人吗"——**不需要的直接降级或删除**。

**反面指标**：如果 `rate(alertmanager_notifications_total[1d])` 远大于真正处理的事故数，说明你的告警在骗人。

---

## 7. 面试高频

**Q1：告警应该基于症状还是基于原因？**
面向值班（page）的必须基于症状（错误率、P99 延迟——用户正在受影响），基于原因的（CPU、磁盘）降级为 ticket 或看板。例外是"即将耗尽"类预测告警（磁盘 4 小时后满、证书 7 天后过期）。原因：微服务故障是网状传播的，一次根因会让几十个原因指标异常，按原因告警会制造噪声并掩盖根因。

**Q2：`for` 字段是干什么的？取值怎么定？**
让告警必须持续满足条件才从 Pending 转 Firing，用于消解瞬时抖动（GC 停顿、发版重启、网络抖动）。取值 ≈ 可接受的告警延迟：核心服务 2~5 分钟，非核心 15 分钟。

**Q3：Alertmanager 的分组、抑制、静默分别是什么？**
分组：按 label 把同一次故障的几十条告警合并成一条通知（`group_wait` 攒 30 秒再发）。抑制：配置里的长期规则，高等级告警发生时静默它引发的低等级告警。静默：运维临时按 label 屏蔽（有过期时间），用于发版窗口或已知问题。

**Q4：为什么 Alertmanager 要等 30 秒才发通知？**
`group_wait`。一次故障会在几秒内打出几十条告警，等 30 秒让它们攒成一条"N 条告警"的通知，把通知数量与故障规模解耦。

**Q5：告警怎么分级？**
critical/page（用户受影响，电话叫醒，5~15 分钟响应）、warning/ticket（异常但无害，群消息，当班处理）、info（只进看板）。判断标准：**能不能等到明天早上**——能等的就不是 page。

**Q6：一条合格的 page 告警要有什么？**
一句话 summary（发生了什么，让人不用开 Grafana 就知道大概）、runbook 链接（怎么办）、影响面（多少用户、哪个功能）。没有这三样的不该进 page 通道。

**Q7：怎么降低告警噪声？**
六步：删掉基于原因的 page 告警（通常砍一半）、加 `for`、配 `group_by`+`group_wait`、配抑制规则、加流量守卫防低流量误报、每季度复盘逐条问"是否真需要叫醒人"。

**Q8：`predict_linear` 用在什么场景？**
预测性告警：用最近一段时间的趋势线性外推，判断未来某个时刻是否会越界（如"4 小时后磁盘满"）。比静态阈值（"剩余 10%"）聪明，因为 10% 对不同容量含义完全不同。

---

本篇对应实验：experiments/11_alerting.go（告警规则求值引擎：表达式求值 → Pending/Firing 状态机 → `for` 抗抖动 → 按 group_by 分组 → 抑制规则生效，打印一次故障从 50 条告警收敛到 1 条通知的全过程）
