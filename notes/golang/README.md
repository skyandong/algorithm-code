# golang 学习笔记

> 目标：以「面试为纲、工程为本」系统掌握 Go——语言机制、并发模型、运行时底层是面试的硬通货，context/错误处理/性能调优是工作的日常。**本文档基于 Go 1.26（go.mod 声明，本机 1.26.3）编写**，所有结论描述当前版本的实际行为，不做历史版本对比；每篇配断言式实验，**结论必须能被 `go test` 复现**。
> **主线认知：** Go 的复杂度都藏在「看起来简单」的语法下面——`go func()` 背后是 GMP 调度器，`append` 背后是扩容拷贝，`err != nil` 背后是接口二元组。学 Go 就是把这些隐含机制变成显性知识。

## 目录

**语言核心篇（01-03）：语言给了你什么**

1. [slice、string 与 map 底层](01-slice、string与map底层.md) — slice/string 头结构、append 扩容的拷贝语义与删除泄漏、string 不可变契约/互转/rune、map Swiss Table/rehash/并发 fatal、unsafe 零拷贝与红线
2. [interface 与反射](02-interface与反射.md) — eface/iface、nil 陷阱、装箱逃逸、reflect 性能姿势
3. [泛型](03-泛型.md) — 类型集与 ~ 约束、GC shape + 字典实现、方法限制、slices/maps 标准库

**并发篇（04-06）：Go 的招牌**

4. [并发内存模型与可见性](04-并发内存模型与可见性.md) — data race、happens-before、编译器与 CPU 重排、Race detector
5. [channel 内部与 nil 语义](05-channel内部与nil语义.md) — hchan 结构、发送/接收六种组合、closed 广播语义
6. [sync 并发原语](06-sync并发原语.md) — Mutex 双模式、RWMutex 闸门、WaitGroup 协议、atomic 边界、Pool/Map/Once 定位

**运行时篇（07-08）：机制背后**

7. [运行时调度器 GMP](07-运行时调度器GMP.md) — G/M/P 职责、work-stealing、抢占、系统调用 hand-off
8. [内存管理与 GC](08-内存管理与GC.md) — 逃逸分析、三色标记、GOGC/GOMEMLIMIT、容器内存

**工程实践篇（09-10）：日常吃饭**

9. [context 与错误处理](09-context与错误处理.md) — Background/TODO 根节点、取消树传播、Cause 取消原因、Value 边界、%w 错误链、panic/recover、defer 陷阱
10. [性能调优实战](10-性能调优实战.md) — benchmark 规范、pprof/trace 闭环、GC 调优、线上排查路径

**考前速刷**

- [面试一口答](面试一口答.md) — **全模块唯一的面试题汇总（66 题）**：考前速刷「张口就来」；各篇正文只留考点结论，面试问答集中在这里

## 重点回顾(自测)

**语言核心**

- [ ] slice 三字段头；append 超 cap = 换新数组 + 搬元素值（浅拷贝，指针仍共享）；共享底层数组的经典坑
- [ ] map 无序的原因；负载因子超限触发 rehash（翻倍或分裂）；并发读写为何 fatal 不可 recover
- [ ] string (ptr,len) 头、无 \0、子串零拷贝；不可变 → 驻留/map key/哈希稳定
- [ ] 循环拼接禁用 `s += x`（O(n²)）；Builder + Grow 是标准答案
- [ ] len(s) 是字节数；range 按码点、索引是字节偏移；按字节截断会碎码
- [ ] `[]byte(s)` 默认拷贝；不逃逸免拷贝、`m[string(b)]` 免分配两个特例
- [ ] unsafe 两类出场（类型双关/零拷贝）；uintptr 只能单表达式内运算；四条红线
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
- [ ] sync.Pool：GC 清空（有 victim 缓冲但不保证留住）、放回前 Reset、定位减 GC 压力
- [ ] sync.Map 两个适用场景 + 何时该用 map+Mutex
- [ ] 五类陷阱：Mutex 不可重入、复制已用过的锁、双检查过早发布、池里的对象不缩容、nil channel 被意外造出

**运行时**

- [ ] GMP：P 的数量=GOMAXPROCS、M 按需创建、work-stealing、信号异步抢占
- [ ] 三色标记 + 混合写屏障；GOGC 语义（下次 GC 目标 = 活跃堆×(1+GOGC/100)）

**工程**

- [ ] context：取消沿树广播；WithTimeout 必须 defer cancel（两重泄漏）
- [ ] %w 成链 %v 断链；errors.Is 哨兵 / As 结构化；禁用字符串判错
- [ ] recover 必须在 defer 体内直接调用；跨 goroutine 拦不住 panic
- [ ] defer：参数立即求值、LIFO、命名返回值可改写、循环内累积
- [ ] 调优闭环：压测复现 → pprof 定位 → benchstat 验证；优化性价比排序

**测试与工程实践**（工程习惯清单；面试不考，不占题库）

- [ ] TDD 红-绿-重构：先看它红、最小实现、忍住不做没被要求的事（YAGNI 执行机制）。边界判据——需求稳定到能写成规格、且这段代码会活过今天，否则先写实现再补测试
- [ ] 表驱动 + t.Run：用例名即规格；错误分支一等公民，errors.Is 对哨兵不比字符串
- [ ] Fatal（前置条件）/ Error（独立断言）分工；t.Helper 报错定位到调用方
- [ ] mock = 消费者侧最小接口 + 手写 stub（记录调用 + 注入失败）；时钟/随机注入函数值
- [ ] 测试分层：CI 纯逻辑打底、`-short` 跳慢用例、集成依赖带守卫（探测不到就 `t.Skip`，requireKafka 模式）；flaky 测试比没有测试更糟
- [ ] 什么时候不 TDD：spike/一次性脚本/UI/并发时序
- [ ] 覆盖率没有及格线：`-coverprofile` 找低覆盖函数看是不是边界漏测，为凑数字写 getter/setter 测试是自欺，真正要盯的是「改到的代码有没有测试保护」

## 跑实验

全部实验都是**断言式单元测试**（无打印输出），用 `go test` 跑：

```bash
cd notes/golang
go test ./experiments/                   # 全部实验
go test -run TestChannel ./experiments/  # 按用例名挑（一篇一个前缀）

# 竞态检测（并发篇必开）：无报告 = 同步原语用对了
go test -race ./experiments/

# 逃逸分析验证（08 篇）：-run '^$' 只编译不跑用例
go test -gcflags="-m -l" -run '^$' ./experiments/ 2>&1 | grep -E "escapes to heap|moved to heap"
```

**文件说明**

| 文件 | 内容 |
|------|------|
| `experiments/NN_xxx_test.go` | 每个实验的全部内容：实现（类型/函数）+ 断言用例，无打印 |
| `go.mod` | 独立 module `agolang`（Go 1.26） |

## 跑测试与写测试

```bash
cd notes/golang
make test        # 全量（约 4s）
make test-unit   # 跳过慢用例（-short，约 1s）
go test ./experiments/ -v     # 等价 make test
go test ./experiments/ -short # 等价 make test-unit
```

`-short` 跳过的是含 sleep、大内存分配或子进程崩溃验证的用例（并发阻塞对比、128MB GC 实验、fatal 子进程等）；其余断言在任何环境都能跑，CI 兜底。

**四个写测试的习惯**（本模块实验全部照此写）：

1. **Fatal vs Error 的分工**：前置条件失败用 `t.Fatalf`（后面全白跑），独立断言用 `t.Errorf`（一次跑出所有失败点）；
2. **`t.Helper()`**：自定义辅助函数开头调用，报错行号指向调用方而不是辅助函数内部；
3. **`t.Cleanup(f)`**：替代「函数尾部手工还原」——临时文件、全局状态复位，中途 `Fatal` 也能执行；
4. **`-run` / `-v` / `-short` 三件套**：`-run 'TestXxx/子用例名'` 单点重跑、`-v` 看每个用例、`-short` 跳慢用例。

**测试分层**（本仓的分法）：纯逻辑用例打底（任何环境秒级跑完，CI 兜底）→ 慢用例挂 `testing.Short()` 守卫 → 靠真依赖的用例先探测、探测不到就 `t.Skip`（akafka 的 `requireKafka` 模式：CI runner 没有 broker 也能全绿，本地起了容器自动升级为真集成测试，同一份代码两种形态）。**CI 挂掉的测试会迅速失去所有人的信任**，所以跑不了的用例要么 Skip、要么单列 job。

## 与其他模块的衔接

- `notes/akafka` — franz-go 客户端：goroutine 生命周期、context 取消、errgroup 并发生产的实战应用
- `notes/redis` / `notes/mysql` — 连接池语义对照 sync.Pool 的「不是连接池」结论
- `notes/nginx` — 边缘代理限流 vs 应用内限流（06 篇实验的 IP 限流手写题是它的应用内对照）
- `algorithms/` — 数据结构实现；Go 底层（01 篇）是面试里「语言内建数据结构」的参考答案
- `algorithms/stack` — slice 版栈的具体类型实现，对照 03 篇 `Stack[T]` 泛型版
- `web/` — HTTP 服务：context 传播与错误码映射的落地场景
- `notes/design-pattern/01` / `05` — 消费者侧接口是「mock 免费」的理论基础（问答口径见 [面试一口答](面试一口答.md)）
- `notes/design-pattern/09` — DDD 领域层零依赖 = 可测试性的验收标准，两篇互为印证

## 延伸阅读（外部资源）

正文的结论已落在对应分册，这里只留出处，不重复摘抄。

| 资源 | 出处 | 看点 |
|------|------|------|
| Channel Axioms | https://dave.cheney.net/2014/03/19/channel-axioms | nil / closed channel 四条公理（05 篇） |
| Curious Channels | https://dave.cheney.net/2013/04/30/curious-channels | close 广播、nil channel 多路等待（05 篇） |
| Never start a goroutine without knowing how it will stop | https://dave.cheney.net/2016/12/22/never-start-a-goroutine-without-knowing-how-it-will-stop | goroutine 生命周期第一原则（design-pattern/06） |
| Practical Go | https://dave.cheney.net/practical-go | 优雅关闭、错误处理、API 设计 |
| Ultimate Go / Ardan Labs Blog | https://www.ardanlabs.com/blog/ | GOMAXPROCS、调度器追踪、GC 三部曲（07、08 篇） |
| Go 并发编程小测验（13 题） | https://colobu.com/2019/04/28/go-concurrency-quizzes/ | 并发陷阱经典题（思路已落进各篇的陷阱实验） |
| Concurrency is not parallelism | https://go.dev/blog/waza-talk | Rob Pike 演讲：并发不是并行（07 篇） |
| Pipelines and cancellation | https://go.dev/blog/pipelines | pipeline 关闭规则与取消传播（design-pattern/06） |
| Go Concurrency Patterns: Context | https://go.dev/blog/context | 取消树的实现与传播 |
| Go Concurrency Patterns: Timing out, moving on | https://go.dev/blog/concurrency-timeouts | select + 超时 |
| Advanced Go Concurrency Patterns | https://go.dev/blog/io2013-talk-concurrency | 高阶并发模式 |
| Share Memory By Communicating | https://go.dev/blog/codelab-share | 用通信共享内存（04 篇） |
| Race Detector | https://go.dev/blog/race-detector | `-race` 的原理与用法 |
| 谢孟军《Go Web 编程》并发章节 | https://github.com/astaxie/build-web-application-with-golang/blob/master/zh/02.7.md | goroutine / channel / select 入门示例 |
| 无闻《Go 编程基础》第 14 课：并发 | https://www.bilibili.com/video/BV15h41187nx/ | 入门视频（47 分钟） |
