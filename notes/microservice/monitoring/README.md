# monitoring：本机可跑的完整监控闭环

对应笔记 07~14 篇。这里不是文档，是**能一键起起来的真实 Prometheus + Grafana + Alertmanager**，
用来验证笔记里的每个说法（抓取、PromQL、告警收敛、看板）都是真的。

## 起停

```bash
docker compose up -d            # 首次会拉镜像, 稍慢
docker compose ps               # 四个容器都 healthy/up
docker compose logs -f prometheus
docker compose down             # 停掉（数据卷不保留, 反正是实验）
```

起完之后：

| 入口 | 地址 | 用途 |
|---|---|---|
| Prometheus | http://localhost:9090 | `/targets` 看抓取状态，`/graph` 手写 PromQL |
| Alertmanager | http://localhost:19093 | 看告警分组与抑制的实际效果（宿主 9093 常被本地 Kafka 占用） |
| Grafana | http://localhost:3000 | `admin/admin`，已自动加载 RED 看板 |
| node-exporter | http://localhost:9100/metrics | 直接 curl 看 exposition 文本 |

## 验证清单（建议按顺序做一遍）

**① 抓到了吗**（08 篇 pull 模型）

```bash
curl -s 'http://localhost:9090/api/v1/targets' | python3 -c \
  "import sys,json;[print(t['labels']['job'], t['scrapeUrl'], t['health']) for t in json.load(sys.stdin)['data']['activeTargets']]"
```

`go-service` 会是 `down`——因为宿主机上还没跑服务（见下）。这正是 `up=0` 的意义：
**抓取失败本身就是存活判定**，这是 pull 模式白送的告警源。

**② PromQL 是不是真的**（09 篇）

```bash
# 主机 CPU 使用率——注意 rate 窗口是 [5m], 而抓取间隔是 15s（窗口 >= 4 倍）
curl -s --data-urlencode 'query=1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) by (instance)' \
  http://localhost:9090/api/v1/query | python3 -m json.tool

# 磁盘 4 小时后会不会满（predict_linear, 10 篇的预测性告警）
curl -s --data-urlencode 'query=predict_linear(node_filesystem_avail_bytes[6h], 4*3600)' \
  http://localhost:9090/api/v1/query | python3 -m json.tool
```

**③ 告警真的会收敛吗**（10 篇）

`go-service` 一直 down，1 分钟后 `TargetDown` 进入 firing：

```bash
curl -s http://localhost:9090/api/v1/alerts | python3 -m json.tool | head -40
curl -s http://localhost:19093/api/v2/alerts  | python3 -m json.tool | head -40
```

对比两边的条数——Prometheus 侧是**每条告警一个对象**，Alertmanager 侧按 `group_by: [job]`
合并后**每个 job 一条通知**。这就是 10 篇 §3 说的"把通知数量与故障规模解耦"。
`experiments/11_alerting` 用纯 Go 复现了同样的收敛过程（50 条 → 1 条）。

**④ 看板能读吗**（11 篇）

Grafana 登录后 → Dashboards → 可观测性 → `RED 服务看板`。把 `job` 变量切到 `node`，
饱和度面板会有数据；切到 `go-service` 全是空——因为还没起服务。

## 把宿主机上的 Go 服务挂进去

`prometheus.yml` 里已经配好了 `go-service` 指向 `host.docker.internal:18080`。
要让它变绿，起仓库里现成的零依赖服务（`monitoring/serve-metrics.go`，会模拟约 300 QPS 的流量）：

```bash
# 终端 A: 起一个持续暴露 /metrics 的 Go 服务（18080 端口, 不用 8080 是避开 kafka-ui）
cd /Users/tal/code/mine/algorithm-code/notes/microservice
go run ./monitoring/serve-metrics.go
```

```bash
# 终端 B: 验证
curl -s http://localhost:18080/metrics | head -20
```

一分钟内 Prometheus 下一轮抓取后 `go-service` 变 up，打开 Grafana 的 RED 看板（job 选 `go-service`）就能看到 QPS/错误率/延迟曲线。

> 这个服务用的是仓库里 `experiments/` 同款的手写指标内核（`atomic` 计数 + 桶累计语义），
> 指标名与 `red-dashboard.json` 的 expr 对齐，所以看板开箱即用，不需要改任何表达式。

## 文件说明

| 文件 | 对应笔记 | 内容 |
|---|---|---|
| `prometheus.yml` | 08 §4 | 三个时间参数、抓取目标、host.docker.internal 回连宿主机 |
| `alert.rules.yml` | 10 §5 | 六条规则：存活、磁盘预测、CPU、错误率、P99、goroutine 泄漏 |
| `alertmanager.yml` | 10 §3 | group_by/group_wait、critical 抑制 warning、分级路由 |
| `grafana/dashboards/red-dashboard.json` | 11 §4 | RED 四行布局：Stat 总览 → QPS/错误率 → 延迟三线 → 饱和度 |
| `grafana/provisioning/` | 11 §1 | 配置即代码，看板进 Git 而不是在 UI 上点 |
