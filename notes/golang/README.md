# golang 学习笔记

> 目标：以「面试为纲、工程为本」系统掌握 Go——语言机制、并发模型、运行时底层是面试的硬通货，context/错误处理/性能调优是工作的日常。所有结论都对齐 Go 1.26 语义，版本分界单独标注；每篇配可运行实验，**结论必须能被 `go run` 复现**。
> **主线认知：** Go 的复杂度都藏在「看起来简单」的语法下面——`go func()` 背后是 GMP 调度器，`append` 背后是扩容拷贝，`err != nil` 背后是接口二元组。学 Go 就是把这些隐含机制变成显性知识。

## 目录

**语言核心篇（01-04）：语言给了你什么**

1. [slice 与 map 底层](01-slice与map底层.md) — slice 头结构/扩容、map 哈希表/扩容搬迁/并发 fatal
2. [string 底层](02-string底层.md) — (ptr,len) 头、不可变契约、互转拷贝与免拷贝特例、rune/UTF-8
3. [interface 与反射](03-interface与反射.md) — eface/iface、nil 陷阱、装箱逃逸、reflect 性能姿势
4. [泛型](04-泛型.md) — 类型集与 ~ 约束、GC shape + 字典实现、方法限制、slices/maps 标准库

**并发篇（05-07）：Go 的招牌**

5. [并发内存可见性与 sync.Once](05-并发内存可见性与sync.Once.md) — data race、happens-before、重排、Once 双检查
6. [Channel 内部与 nil 语义](06-Channel内部与nil语义.md) — hchan 结构、发送/接收六种组合、closed 广播语义
7. [sync 锁与原子操作](07-sync锁与原子操作.md) — Mutex 双模式、RWMutex 闸门、WaitGroup 协议、atomic 边界、Pool/Map 定位

**运行时篇（08-09）：机制背后**

8. [运行时调度器 GMP](08-运行时调度器GMP.md) — G/M/P 职责、work-stealing、抢占、系统调用 hand-off
9. [内存管理与 GC](09-内存管理与GC.md) — 逃逸分析、三色标记、GOGC/GOMEMLIMIT、容器内存

**工程实践篇（10-11）：日常吃饭**

10. [context 与错误处理](10-context与错误处理.md) — 取消树传播、Value 边界、%w 错误链、panic/recover、defer 陷阱
11. [性能调优实战](11-性能调优实战.md) — benchmark 规范、pprof/trace 闭环、GC 调优、线上排查路径

**面试冲刺篇（12-14）**

12. [Goroutine 面试题集](12-Goroutine面试题集.md) — 手写题 + 选择题 + 简答，19 题全解
13. [名家并发模式汇总](13-名家并发模式汇总.md) — Dave Cheney/Kennedy/鸟窝等博客的实战模式沉淀
14. [面试一口答](面试一口答.md) — 考前速刷：高频问题「张口就来」

## 重点回顾(自测)

**语言核心**

- [ ] slice 三字段头；append 扩容策略；共享底层数组的经典坑
- [ ] map 无序的原因；扩容渐进搬迁；并发读写为何 fatal 不可 recover
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

- [ ] GMP：P 的数量=GOMAXPROCS、M 按需创建、work-stealing、Go 1.14 信号抢占
- [ ] 三色标记 + 混合写屏障；GOGC 语义（下次 GC 目标 = 活跃堆×(1+GOGC/100)）

**工程**

- [ ] context：取消沿树广播；WithTimeout 必须 defer cancel（两重泄漏）
- [ ] %w 成链 %v 断链；errors.Is 哨兵 / As 结构化；禁用字符串判错
- [ ] recover 必须在 defer 体内直接调用；跨 goroutine 拦不住 panic
- [ ] defer：参数立即求值、LIFO、命名返回值可改写、循环内累积
- [ ] 调优闭环：压测复现 → pprof 定位 → benchstat 验证；优化性价比排序

## 跑实验

```bash
cd notes/golang
go run ./experiments/ all        # 全部实验
go run ./experiments/ sync       # 单跑某个：07 篇
go run ./experiments/ string     # 02 篇
# 可用名: visibility|channel|interview|masters|gmp|gcmemory|slicemap|interface|sync|context|performance|string|generics

# 竞态检测（并发篇必开）
go run -race ./experiments/ visibility

# 逃逸分析验证（09 篇）
go build -gcflags="-m -l" ./experiments/ 2>&1 | grep -E "escapes|moved to heap"
```

**文件说明**

| 文件 | 内容 |
|------|------|
| `experiments/NN_*.go` | 每篇笔记对应的可运行验证代码，`第N节` 与笔记章节对齐 |
| `experiments/main.go` | 实验分发入口，`go run ./experiments/ <名字>` |
| `go.mod` | 独立 module `agolang`（Go 1.26） |

## 与其他模块的衔接

- `notes/akafka` — franz-go 客户端：goroutine 生命周期、context 取消、errgroup 并发生产的实战应用
- `notes/redis` / `notes/mysql` — 连接池语义对照 sync.Pool 的「不是连接池」结论
- `notes/nginx` — 边缘代理限流 vs 应用内限流（12 篇手写 IP 限流的上游对照）
- `algorithms/` — 数据结构实现；Go 底层（01 篇）是面试里「语言内建数据结构」的参考答案
- `algorithms/stack` — slice 版栈的具体类型实现，对照 04 篇 `Stack[T]` 泛型版
- `web/` — HTTP 服务：context 传播与错误码映射的落地场景
