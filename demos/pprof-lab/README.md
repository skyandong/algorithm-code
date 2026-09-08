# pprof-lab：Go 排障实战场（配套 notes/golang/11）

一个**故意带病**的服务 + 工程标准姿势的 pprof 工作流。所有工具都是 Go 自带的，不引入任何第三方——
这就是线上排查时你真正会用的那套东西。

## 病灶一览（对应面试/线上的高频根因）

| 病灶 | 触发 | pprof 里长什么样 | 正确修法 |
|---|---|---|---|
| ① 热路径重复编译正则 | 自动负载 + `GET /cpu` | `regexp.Compile` cum 占大头 | 包级 `var re = regexp.MustCompile(...)` 编译一次 |
| ② 锁竞争（临界区干慢活） | 自动负载 + `GET /lock` | block/mutex profile 里 `sync.Mutex.Lock` | 缩小临界区，慢操作挪出锁外 |
| ③ goroutine 泄漏 | 自动负载 + `GET /leak` | goroutine 数只涨不跌，栈卡在 channel 发送 | 带 buffer/context 取消/select 退出 |
| ④ 内存泄漏（slice 截取） | 自动负载 + `GET /mem` | heap inuse 持续涨，`main.leakMemory` | `append([]byte{}, big[:16]...)` 拷贝出来 |

对照组：`/hash` 是**正当热点**（真的在干活），用来练习区分"该优化的"和"不该动的"。
彩蛋：`regexBacktrack` 演示 **Go 的 regexp 是 RE2 引擎，线性时间，免疫灾难性回溯**——
`(a+)+$` 在 Java/Python 里是指数爆炸，在 Go 里只要几微秒。Go 服务 CPU 打满时，
正则的真实问题从来不是"回溯"，而是像病灶①那样在请求里反复 Compile。

## 5 分钟上手（标准工作流）

```bash
# ① 起服务（业务 :8081, pprof :6060 仅 localhost —— 生产规矩: pprof 无鉴权不能裸奔公网）
go run .

# ② 抓 30s CPU profile 并直接打开 Web UI（工程里 99% 的时候就是这么用的）
go tool pprof -http=:8083 'http://localhost:6060/debug/pprof/profile?seconds=30'
#    → 浏览器自动打开, 左上 VIEW 菜单:
#      Top        先看排行
#      Graph      调用图（粗看流向）
#      Flame Graph ★ 火焰图——根在顶叶子在底, 越宽占比越大, 点任意帧下钻
#      Source     ★ 行级耗时（本仓库源码在本地, 直接看到哪一行烧的）

# ③ 服务别停, 再抓内存（对照着看 CPU 大头是不是分配导致的）
go tool pprof -http=:8084 -sample_index=alloc_space 'http://localhost:6060/debug/pprof/heap'
```

本仓库已预抓好一套产物（`profiles/`），**不起服务也能玩**：

```bash
go tool pprof -http=:8083 profiles/cpu.pb.gz          # CPU, 抓取时 330% CPU 密度
go tool pprof -http=:8083 -sample_index=inuse_space profiles/heap.pb.gz   # 当前存活
go tool pprof -http=:8083 -sample_index=alloc_space profiles/heap.pb.gz   # 累计分配（GC 压力源）
go tool pprof -http=:8083 profiles/mutex.pb.gz        # 锁争用
```

## 预抓产物里已经能看到什么（对着找）

先看终端版排行（不想开浏览器时）：

```bash
go tool pprof -top -cum profiles/cpu.pb.gz
```

| 画像 | 你应该看到的 |
|---|---|
| `cpu.pb.gz`（cum 视角） | `main.hashLoop` ~62%（正当热点），**`regexp.Compile` ~13%**——其中 75% 的时间花在 Compile 而不是 Match，这就是"编译贵于匹配"的实证 |
| `heap.pb.gz`（alloc_space） | `main.leakMemory`（1MB 底层数组被 16B 引用钉住）+ `regexp/syntax` 大量分配（每次 Compile 的垃圾都在喂 GC） |
| `heap.pb.gz`（inuse_space） | 持续增长的 `memorySink`——抓两次对比更能看出涨势 |
| `goroutine.txt` | 979+ 个 goroutine 全部卡在 `main.go:125` 的 `leakedCh <- struct{}{}` |
| `block.pb.gz` | `sync.Mutex.Lock` 数十秒等待（病灶②） |
| `mutex.pb.gz` | 持锁者 `lockContend` 的争用记录 |

## 定位到函数之后：三个动作

```bash
# 行级定位（需要本地源码, 本仓库就在本地, 直接可用）
go tool pprof -list regexOnHotPath profiles/cpu.pb.gz

# 看谁调用它 / 它调用了谁（快速确认嫌疑链）
go tool pprof -peek 'regexp.Compile' profiles/cpu.pb.gz

# 不开浏览器的纯终端火焰图替代: 树状视图
go tool pprof -tree profiles/cpu.pb.gz | head -30
```

## 生产姿势：线上怎么抓（不是本机这么方便）

```bash
# pprof 端口没暴露到公网时, 用端口转发把远程 6060 映射到本地
kubectl port-forward pod/pay-service-xxx 6060:6060
go tool pprof -http=:8083 'http://localhost:6060/debug/pprof/profile?seconds=30'

# 或者先把 profile 文件抓下来, 回到本地离线分析（不留服务器上的常驻连接）
curl -s 'http://pod:6060/debug/pprof/profile?seconds=30' -o /tmp/cpu.pb.gz
curl -s 'http://pod:6060/debug/pprof/heap?gc=1'         -o /tmp/heap1.pb.gz
# …10 分钟后…
curl -s 'http://pod:6060/debug/pprof/heap?gc=1'         -o /tmp/heap2.pb.gz
go tool pprof -base /tmp/heap1.pb.gz /tmp/heap2.pb.gz   # ★ 增量视角: 谁在涨一目了然
```

## 命令速查

| 场景 | 命令 |
|---|---|
| CPU 热点 | `go tool pprof -http=:8083 'http://localhost:6060/debug/pprof/profile?seconds=30'` |
| 内存泄漏（当前存活） | `.../debug/pprof/heap`（默认 inuse_space） |
| GC 压力来源（累计分配） | `-sample_index=alloc_space` |
| goroutine 泄漏点 | `curl '.../goroutine?debug=1'`（聚合栈）或 `?debug=2`（全量细节含等待时长） |
| 锁等待 | `runtime.SetBlockProfileRate(10_000)` 开启后抓 `.../block` |
| 锁争用（持锁方视角） | `runtime.SetMutexProfileFraction(5)` 开启后抓 `.../mutex` |
| 两个时刻 diff | `go tool pprof -base old.pb.gz new.pb.gz` |
| 行级耗时 | `go tool pprof -list <FuncRegex> <profile>` |

## 已知注意点

- pprof 的 `profile` 端点是**同步阻塞**的：请求多少秒就采样多少秒，抓取期间服务照常跑。
- `heap?gc=1` 先强制 GC 再快照——看"真实存活"必须加，否则全是还没来得及 GC 的垃圾。
- 本服务负载刻意设计为 **~330% CPU（约 3.3 核）**，配比调过：火焰图上能看到两根主柱
  （正当热点 + 病灶①）。Mac 核多，不会把你机器打满；但别在生产环境照抄这个自动负载。
- 泄漏类病灶是**缓慢注入**的（每 0.8s 一针），抓 heap/goroutine 前让服务多跑一会儿，效果更明显。
