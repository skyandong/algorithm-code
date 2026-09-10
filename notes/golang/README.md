# golang 学习笔记

> 目标：以「面试为纲、工程为本」系统掌握 Go——语言机制、并发模型、运行时底层是面试的硬通货，context/错误处理/性能调优是工作的日常。**本文档基于 Go 1.26（go.mod 声明，本机 1.26.3）编写**，所有结论描述当前版本的实际行为，不做历史版本对比；每篇配可运行实验，**结论必须能被 `go run` 复现**。
> **主线认知：** Go 的复杂度都藏在「看起来简单」的语法下面——`go func()` 背后是 GMP 调度器，`append` 背后是扩容拷贝，`err != nil` 背后是接口二元组。学 Go 就是把这些隐含机制变成显性知识。

## 目录

**语言核心篇（01-03）：语言给了你什么**

1. [slice、string 与 map 底层](01-slice与map底层.md) — slice/string 头结构、slice 扩容与删除泄漏、string 不可变契约/互转/rune、map Swiss Table/rehash/并发 fatal
2. [interface 与反射](02-interface与反射.md) — eface/iface、nil 陷阱、装箱逃逸、reflect 性能姿势
3. [泛型](03-泛型.md) — 类型集与 ~ 约束、GC shape + 字典实现、方法限制、slices/maps 标准库

**并发篇（04-06）：Go 的招牌**

4. [并发内存可见性与 sync.Once](04-并发内存可见性与sync.Once.md) — data race、happens-before、重排、Once 双检查
5. [Channel 内部与 nil 语义](05-Channel内部与nil语义.md) — hchan 结构、发送/接收六种组合、closed 广播语义
6. [sync 锁与原子操作](06-sync锁与原子操作.md) — Mutex 双模式、RWMutex 闸门、WaitGroup 协议、atomic 边界、Pool/Map 定位

**运行时篇（07-08）：机制背后**

7. [运行时调度器 GMP](07-运行时调度器GMP.md) — G/M/P 职责、work-stealing、抢占、系统调用 hand-off
8. [内存管理与 GC](08-内存管理与GC.md) — 逃逸分析、三色标记、GOGC/GOMEMLIMIT、容器内存

**工程实践篇（09-10）：日常吃饭**

9. [context 与错误处理](09-context与错误处理.md) — 取消树传播、Value 边界、%w 错误链、panic/recover、defer 陷阱
10. [性能调优实战](10-性能调优实战.md) — benchmark 规范、pprof/trace 闭环、GC 调优、线上排查路径

**测试篇（13）：质量是设计出来的**

13. [TDD 测试驱动开发](13-TDD测试驱动开发.md) — 红-绿-重构循环、表驱动与子测试、手写 stub 与消费者侧接口、测试金字塔与守卫模式

**面试冲刺篇（11-12）**

11. [Goroutine 面试题集](11-Goroutine面试题集.md) — 手写题 + 选择题 + 简答，19 题全解
12. [名家并发模式汇总](12-名家并发模式汇总.md) — Dave Cheney/Kennedy/鸟窝等博客的实战模式沉淀
- [面试一口答](面试一口答.md) — 考前速刷：高频问题「张口就来」

## 重点回顾(自测)

**语言核心**

- [ ] slice 三字段头；append 扩容策略；共享底层数组的经典坑
- [ ] map 无序的原因；负载因子 7/8 触发 rehash（翻倍或分裂）；并发读写为何 fatal 不可 recover
- [ ] string (ptr,len) 头、无 \0、子串零拷贝；不可变 → 驻留/map key/哈希稳定
- [ ] 循环拼接禁用 `s += x`（O(n²)）；Builder + Grow 是标准答案
- [ ] len(s) 是字节数；range 按码点、索引是字节偏移；按字节截断会碎码
- [ ] `[]byte(s)` 默认拷贝；不逃逸免拷贝、`m[string(b)]` 免分配两个特例
- [ ] interface 二元组；nil != nil 的原理；方法集 T vs *T
- [ ] 泛型：~ 近似约束放行 `type UserID int`；comparable 是「== 可用」的类型集
- [ ] GC shape + 字典：指针类型共享一份机器码，非 C++ 全展开也非 Java 全擦除
- [ ] 方法不能有类型参数（接口满足性不可判定）→ 包级泛型函数绕法
- [ ] 泛型 vs 接口：算法对多类型用泛型（零装箱），运行时多态/异构用接口

**并发**

- [ ] data race 定义三要素 + 为什么 `for !ready {}` 可能死循环（可见性）
- [ ] channel 六种读写组合的行为表 + 关闭是广播、发送会丢
- [ ] goroutine 泄漏三大来源（阻塞收发、忘记 cancel、子 goroutine panic）
- [ ] Mutex 正常/饥饿模式切换条件；Unlock 无属主；未加锁 Unlock 是 fatal
- [ ] RWMutex：写锁排队后新读者被挡；不能升级能降级；何时比 Mutex 慢
- [ ] atomic 只保护一个字；无原子浮点；atomic.Pointer[T] 配置快照
- [ ] sync.Pool：GC 清空（victim 两代）、放回前 Reset、定位减 GC 压力
- [ ] sync.Map 两个适用场景 + 何时该用 map+Mutex

**运行时**

- [ ] GMP：P 的数量=GOMAXPROCS、M 按需创建、work-stealing、信号异步抢占
- [ ] 三色标记 + 混合写屏障；GOGC 语义（下次 GC 目标 = 活跃堆×(1+GOGC/100)）

**工程**

- [ ] context：取消沿树广播；WithTimeout 必须 defer cancel（两重泄漏）
- [ ] %w 成链 %v 断链；errors.Is 哨兵 / As 结构化；禁用字符串判错
- [ ] recover 必须在 defer 体内直接调用；跨 goroutine 拦不住 panic
- [ ] defer：参数立即求值、LIFO、命名返回值可改写、循环内累积
- [ ] 调优闭环：压测复现 → pprof 定位 → benchstat 验证；优化性价比排序

**测试**

- [ ] TDD 红-绿-重构：先看它红、最小实现、忍住不做没被要求的事（YAGNI 执行机制）
- [ ] 表驱动 + t.Run：用例名即规格；错误分支一等公民，errors.Is 对哨兵不比字符串
- [ ] Fatal（前置条件）/ Error（独立断言）分工；t.Helper 报错定位到调用方
- [ ] mock = 消费者侧最小接口 + 手写 stub（记录调用 + 注入失败）；时钟/随机注入函数值
- [ ] 测试分层：CI 纯逻辑打底、-short 跳慢用例、集成依赖带守卫（requireKafka 模式）
- [ ] 什么时候不 TDD：spike/一次性脚本/UI/并发时序；覆盖率是探针不是 KPI

## 跑实验

```bash
cd notes/golang
go run ./experiments/ all        # 全部实验
go run ./experiments/ sync       # 单跑某个：06 篇
# 可用名: visibility|channel|interview|masters|gmp|gcmemory|interface|sync|context|performance|generics|tdd
# 注：01 篇（slice/map）与 02 篇（string）已全部改为断言式单测验证，见
#     experiments/01-1_slice_test.go、01-2_map_test.go、02_string_test.go

# 竞态检测（并发篇必开）
go run -race ./experiments/ visibility

# 逃逸分析验证（08 篇）
go build -gcflags="-m -l" ./experiments/ 2>&1 | grep -E "escapes|moved to heap"
```

**文件说明**

| 文件 | 内容 |
|------|------|
| `experiments/NN_*.go` | 每篇笔记对应的可运行验证代码，`第N节` 与笔记章节对齐 |
| `experiments/NN_*_test.go` | 每个实验的单元测试：纯逻辑用例 + demo 冒烟（断言关键输出） |
| `experiments/01-1_slice_test.go`、`01-2_map_test.go`、`02_string_test.go` | 纯断言式验证（无 print），用指针/内存统计/子进程把底层行为钉死 |
| `experiments/main.go` | 实验分发入口，`go run ./experiments/ <名字>` |
| `go.mod` | 独立 module `agolang`（Go 1.26） |

## 跑测试（格式同 akafka 模块）

```bash
cd notes/golang
make test        # 全量：纯逻辑 + demo 冒烟（约 12s）
make test-unit   # 只跑纯逻辑（-short，约 1s）
go test ./experiments/ -v     # 等价 make test
go test ./experiments/ -short # 等价 make test-unit
```

测试分两类：**纯逻辑用例**直接断言实验里的可复用函数（泛型工具、blockingMap、WaitTimeout 等），任何环境可跑，CI 兜底；**demo 冒烟用例**完整跑一遍实验并断言关键结论输出（如 `部分删除`、`close of closed channel`），防止笔记结论与实验代码脱节，`-short` 时跳过。

## 与其他模块的衔接

- `notes/akafka` — franz-go 客户端：goroutine 生命周期、context 取消、errgroup 并发生产的实战应用
- `notes/redis` / `notes/mysql` — 连接池语义对照 sync.Pool 的「不是连接池」结论
- `notes/nginx` — 边缘代理限流 vs 应用内限流（11 篇手写 IP 限流的上游对照）
- `algorithms/` — 数据结构实现；Go 底层（01 篇）是面试里「语言内建数据结构」的参考答案
- `algorithms/stack` — slice 版栈的具体类型实现，对照 03 篇 `Stack[T]` 泛型版
- `web/` — HTTP 服务：context 传播与错误码映射的落地场景
- `notes/design-pattern/01` — 消费者侧接口是 TDD「mock 免费」的理论基础（13 篇第 5 节）
- `notes/design-pattern/09` — DDD 领域层零依赖 = 可测试性的验收标准，两篇互为印证
