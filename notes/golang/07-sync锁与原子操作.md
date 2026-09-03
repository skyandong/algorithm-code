# sync 锁与原子操作

> **核心认知：** Go 的锁是「**信号量排队 + 双模式调度策略**」，不是操作系统教科书里的原始互斥量——Mutex 内部维护等待队列，但**让不让新来的插队**是一个调度策略问题（正常模式 vs 饥饿模式）。atomic 是 Lock-free 的地基（CAS 单条 CPU 指令），但只能保护单个机器字。选型口诀：**一个字用 atomic，一段临界区用 Mutex，读远多于写才考虑 RWMutex，跨 goroutine 传递数据/所有权用 channel**。锁保护的是「不变量」（invariant），不是「代码」——想清楚哪些数据在什么约束下必须一致，锁的边界自然就出来了。

按 Go 1.26 语义说明，版本分界处单独标注。前置知识：happens-before 与 data race 见 `05-并发内存可见性与sync.Once.md`，channel 与并发模式见 `06`/`13`。

---

## 目录

1. [Mutex：状态机与两种模式](#1-mutex状态机与两种模式)
2. [自旋：忙等的适用条件](#2-自旋忙等的适用条件)
3. [RWMutex：读写锁的真实成本](#3-rwmutex读写锁的真实成本)
4. [WaitGroup：三方法协议](#4-waitgroup三方法协议)
5. [atomic：CAS 与内存序](#5-atomiccas-与内存序)
6. [sync.Pool：对象复用的正确姿势](#6-syncpool对象复用的正确姿势)
7. [sync.Map：为读多写少而生](#7-syncmap为读多写少而生)
8. [选型决策表](#8-选型决策表)
9. [面试高频](#9-面试高频)

---

## 1. Mutex：状态机与两种模式

### 1.1 state 字段：一个 int32 装下全部状态

```go
// sync/mutex.go（简化）
const (
    mutexLocked = 1 << iota // 1: 已锁定
    mutexWoken              // 2: 有 goroutine 被唤醒
    mutexStarving           // 4: 饥饿模式
    mutexWaiterShift = iota // 3: waiter 数从第 3 位开始
)

type Mutex struct {
    state int32      // locked | woken | starving | waiterCount<<3
    sema  uint32      // 信号量，阻塞/唤醒排队用
}
```

Lock 的快路径就是一次 CAS：`CompareAndSwapInt32(&m.state, 0, mutexLocked)`——无人竞争时加锁不进内核、不进调度器，和一次 atomic 操作同价。慢路径才会 `semacquire` 排队睡眠。

### 1.2 正常模式 vs 饥饿模式（Go 1.9+）

| | 正常模式（默认） | 饥饿模式 |
|---|---|---|
| 新来者 | 可与被唤醒的 waiter **竞争**，甚至直接抢到（已在 CPU 上，占优） | 锁**直接交接**给队头 waiter，新来者不自旋也不抢，老实排队 |
| 延迟 | 吞吐高，但 waiter 可能一直抢输（尾部延迟差） | waiter 最多等 1ms 就一定拿到 |
| 切换条件 | waiter 等待 **超过 1ms** → 切饥饿 | waiter 拿到锁时发现等待 <1ms 或已是队尾 → 切回正常 |

WHY：纯 FIFO 交接吞吐差（每次交接要完整走一遍调度器唤醒）；纯竞争则不公平（极端情况下队头饿死）。双模式是吞吐与公平的折中——**默认偏向吞吐，检测到饥饿时自动切公平**。这是面试里「Mutex 公平吗」的标准答案：非绝对公平，饥饿时退化为公平。

### 1.3 Unlock 的三个冷知识

1. **Unlock 无属主校验**——`Unlock()` 只是 CAS 把 locked 位清零再发信号，根本不记录谁加的锁。所以「A 加锁、B 解锁」在语义上合法（但强烈不推荐，review 必须打回）；
2. **未加锁就 Unlock 直接 panic**（`fatal: sync: unlock of unlocked mutex`）——这是 fatal，**不可 recover**；
3. **defer mu.Unlock() 是常见写法但不是免费午餐**：函数很长时临界区被拉大到整个函数体。工程建议：锁的粒度以「不变量」为单位，进出临界区后尽早解锁。

```go
// 经典反面教材：临界区被 defer 撑大
func (s *Store) Get(k string) *Item {
    s.mu.Lock()
    defer s.mu.Unlock()      // 拿到 item 后其实就可以解锁了
    item := s.m[k]
    return s.deepCopy(item)  // 拷贝是纯 CPU 工作，却一直持锁
}
```

### 1.4 Mutex 不可复制

Mutex 是「带状态的协调原语」：复制那一刻锁的字段被快照，两个副本各自维护 sema 队列，互斥性彻底失效。`go vet` 有 copylocks 检查会报错。连带效应：**包含 Mutex 的结构体也不可复制**、不能做 map 的 value 被整体读出修改再放回（那是副本）。需要「锁 + 被保护数据」一起用时，让结构体只以指针传播。

---

## 2. 自旋：忙等的适用条件

被唤醒的 waiter 在真正睡眠前会先**自旋**（active spinning）几次，不断尝试 CAS 抢锁。进入自旋的硬条件（全部满足）：

1. 多核机器（单核自旋是纯浪费——让出 CPU 才能让持锁者跑）；
2. `GOMAXPROCS > 1` 且本 P 的本地队列为空（否则自旋反而挡住持锁者）；
3. 自旋次数不超过 4 次；
4. 且仅正常模式（饥饿模式禁止自旋）。

```go
// sync/mutex.go 的判定（简化）
func sync_runtime_canSpin(i int) bool {
    p := gomaxprocs
    if ncpu != p || ... { return false }  // 单 P 不自旋
    if i >= active_spin { return false }  // 最多 4 次
    if p := runtime.Gosched...            // 本地队列非空不自旋
}
```

WHY 自旋有意义：持锁者的临界区往往极短（几十 ns），唤醒-睡眠一次的成本（微秒级，走 runtime 调度器）远高于空转几次。这是「**赌临界区很快结束**」的优化。工程推论：临界区越长，Mutex 越亏——长临界区要么拆（减小锁粒度），要么换模型（shard/分片、读写分离）。

---

## 3. RWMutex：读写锁的真实成本

### 3.1 实现核心：一个 int64 记账

```go
type RWMutex struct {
    w           Mutex      // 写锁互斥（也是写者间排队用）
    writerSem   uint32     // 写者等待信号量
    readerSem   uint32     // 读者等待信号量
    readerCount atomic.Int64 // >0: 读着的人数；<0: 写者在等（此时绝对值=排队读者数）
    readerWait  atomic.Int64 // 写者离场前还需等待的读者数
}
```

读锁：`readerCount.Add(1)`，若结果 <0（有写者在排队）→ 睡在 readerSem 上。
写锁：`readerCount.Add(-rwmutexMaxReaders)` 把计数打为负——**这一刻起新读者全部入睡**；再等 `readerWait` 归零（存量读者全部退出）才真正拿到写锁。

三个常考推论：

1. **写锁等待期间，新读者被阻塞**（不是「读完这批就放写者，后面读者继续进」）——写者不会饿死，靠的就是 readerCount 变负这个「闸门」；
2. **锁不可升级**：持读锁时再 Lock() 必死锁（自己等待自己释放，readerCount 永不归零）。降级可以：持写锁 → RLock() → RUnlock() → Unlock()，顺序不能乱；
3. RLock 嵌套 + 中途有写者插入 → 内层 RLock 阻塞 → 外层 RUnlock 永远等不到 → **读锁重入也是死锁经典款**。

### 3.2 RWMutex 何时反而更慢

RWMutex 的每个操作都要多维护两个信号量和 atomic 计数，单次开销约为 Mutex 的 1.5~2 倍。只有**读临界区耗时长 + 读并发高 + 写极少**时才赚。经验值：

- 读临界区 < 100ns（比如只是读一个 int）：直接 Mutex，RWMutex 是负优化；
- 读临界区是遍历大 map/深拷贝等微秒级工作、读写比 > 10:1：RWMutex 显著赚；
- 中间地带：**先 benchmark 再换**（见 `11-性能调优实战.md` 的总纲）。

工程红线：RWMutex 保护的数据在读临界区里必须真的只读。「读」方法里顺手 put 一个缓存/写一个统计字段，是 data race 高发区——`go test -race` 必开。

---

## 4. WaitGroup：三方法协议

### 4.1 协议与两个 fatal

```go
wg.Add(1)      // 计数 +1（必须在 goroutine 外做，见下）
go func() {
    defer wg.Done() // 计数 -1
    ...
}()
wg.Wait()      // 阻塞到计数归零
```

两条 fatal 红线（都不可 recover）：

1. **Add 的正数发生在 Wait 之后、计数已为 0 时** → `panic: sync: WaitGroup misuse: Add called concurrently with Wait`。这就是「Add 必须在启动 goroutine 的那个 goroutine 里做」的原因——如果放到子 goroutine 里再 Add，主流程可能已经 Wait 了；
2. **计数被减成负数**（Done 多于 Add）→ `panic: sync: negative WaitGroup counter`。

### 4.2 重用（reuse）是支持的，但有窗口期

WaitGroup 计数归零、Wait 返回后，可以重新 Add 复用。但「归零」到「Wait 完全返回」之间有竞态窗口：此时并发 Add 会触发 misuse panic；上一个 Wait 还没返回就开始下一轮 Wait 也是 panic（`WaitGroup is reused before previous Wait has returned`）。并发循环任务用 **channel 或下一节的 errgroup** 更稳。

### 4.3 WaitGroup 没有「带超时的 Wait」

标准库故意不给。工程实现是 `context` + goroutine 竞速（见 `12-Goroutine面试题集.md` 题 5 的手写），或直接用 `golang.org/x/sync/errgroup` 的 `ctx` 版本——后者同时解决「错误收集 + 取消传播 + 并发上限」三件事，是生产代码的正解：

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(8)                 // 并发上限（信号量）
for _, task := range tasks {
    task := task
    g.Go(func() error {
        return handle(ctx, task) // 返回 error 会被收集；首个 error 触发 ctx 取消
    })
}
err := g.Wait() // 返回第一个非 nil error
```

---

## 5. atomic：CAS 与内存序

### 5.1 atomic 操作的能力边界

atomic 包提供**单个机器字的原子读写与 RMW**（read-modify-write）：

| 操作 | 语义 | 典型用途 |
|---|---|---|
| Load / Store | 原子读/写 | 配置热更新、开关位 |
| Add / Sub / And / Or / Xor | 原子算术逻辑 | 计数器（不支持浮点！） |
| CompareAndSwap | `if *addr == old { *addr = new; true }` | 无锁结构的状态迁移 |
| Swap | 无条件换值 | 单槽任务队列 |
| atomic.Value / Pointer[T] | 原子换「任意类型/指针」 | 整体配置快照 |

能力边界（必须背下来）：

1. **只保护一个字**。「用两个 atomic int 表示一个二元状态」是经典错误——两次 atomic 之间没有原子性，中间态会被观察到。要么合成一个字（位运算/把两个 int32 打包进一个 int64/atomic.Pointer 换整个结构体），要么上锁；
2. **没有原子浮点**。`atomic.AddInt64` 不接受 float，需要时用 `atomic.Pointer[float64]` CAS 整体换，或 `math.Float64bits` 打包；
3. **atomic.Value 的 Store 必须存同一个具体类型**（nil 会 panic、类型变了会 panic）。Go 1.19+ 用泛型 `atomic.Pointer[T]` 替代，类型安全且可存 nil；

```go
// 配置热更新的标准姿势：整个配置一次性换指针
type Config struct { Timeout time.Duration; Retries int }
var cfg atomic.Pointer[Config]

cfg.Store(&Config{...})                 // 写端：整体替换
c := cfg.Load()                         // 读端：拿到的是完整快照，字段间天然一致
```

### 5.2 false sharing（伪共享）

atomic 变量只保证「正确」，不保证「快」。多个 CPU 核各自缓存同一 cache line（64B），不同 goroutine 写**不同但相邻**的 atomic 变量时，cache line 互相失效，性能断崖。benchmark 千万级计数时如果发现多核不升反降，先怀疑它。解法：pad 到 cache line 边界（`sync/atomic` 内部就是这么做的，自己写多计数器时同理，间隔 64B）。

### 5.3 内存序：Go 只有一种

C++/Java 有 acquire/release 等多种内存序，Go 故意只暴露一种：**Go 内存模型的同步语义**（本质是顺序一致的 happens-before）。`atomic` 操作之间、atomic 与 mutex/channel 之间都建立 happens-before。

工程上记住推论就够：**一次 atomic.Store 发布数据 + 别的 goroutine atomic.Load 到同一位置后，Store 之前的所有普通写对新读者可见**（release/acquire 效应）。这也是 `05` 篇「用 atomic.Bool 替代裸 bool 做 ready 标志」能修复可见性问题的原因。

```go
var ready atomic.Bool
var data int

// writer
data = 42
ready.Store(true)          // 1️⃣

// reader
if ready.Load() {          // 2️⃣ 读到 true 后
    _ = data               // 3️⃣ 保证看到 42（1️⃣ happens-before 2️⃣，2️⃣ happens-before 3️⃣）
}
```

---

## 6. sync.Pool：对象复用的正确姿势

### 6.1 语义：缓存，不是容器

```go
var bufPool = sync.Pool{
    New: func() any { return new(bytes.Buffer) },
}

// 使用方
b := bufPool.Get().(*bytes.Buffer)
defer func() {
    b.Reset()              // 放回前必须清理，否则下个使用者读到脏数据
    bufPool.Put(b)
}()
b.Write(...)
```

三条必须刻在脑子里的语义：

1. **Pool 里的对象可能在任意时刻被丢弃**（没有「一定命中」的保证）——Get 拿不到就 New，所以 New 必须提供；
2. **每次 GC（STW 的两轮标记扫描）都会清空 Pool**（Go 1.13 起分主/victim 两代：GC 后主代降级为 victim，再下次 GC 才真正丢弃——给对象两个 GC 周期的存活机会，减轻「GC 一来缓存全光」的抖动）；
3. **Put 之后再 Get 可能拿到同一个对象**——所以放回前必须 Reset，状态残留是最高频事故。

### 6.2 工程定位

sync.Pool 解决的是「**高频率、短生命周期、分配成本高**的对象反复分配带来的 GC 压力」，典型：bytes.Buffer（序列化/网络包拼装）、gzip writer、大切片。它**不是**：

- 连接池（连接是资源不是内存，该用 database/sql、pool 库）；
- 缓存（GC 就清了，跨请求语义不保）；
- free list（没有容量、没有淘汰策略）。

一句话：Pool 换的是 **GC 压力**，不是「省一次分配」——先有 benchmark 证明分配是热点，再上 Pool（见 `11` 篇优化性价比排序：sync.Pool 排在逃逸治理之后）。

---

## 7. sync.Map：为读多写少而生

### 7.1 结构：读快写的双 map

```go
// Go 1.23 及以前的实现（读路径零锁的来源）
type Map struct {
    mu     Mutex
    read   atomic.Pointer[readOnly] // 只读 map + amended 标志
    dirty  map[any]*entry           // 含新写入的完整数据
    misses int                       // read 未命中次数，满阈值(dirty长度)把 dirty 升级为 read
}
```

- **读**：先走 read（atomic 加载，无锁），命中即返回——这是它快的唯一原因；
- **写新 key**：拿 mu 写 dirty；read 里没有的 key 读写都要走 dirty，misses 攒够后 dirty 晋升为新 read（旧的进垃圾）；
- **删**：打 nil 标记（惰性删除），不立即回收 entry。

### 7.2 版本分界：Go 1.24 换成 HashTrieMap

Go 1.24 起 sync.Map 内部改为**哈希 trie（HAMT 变体）**：写不再走「双 map + 整体晋升」，而是按 hash 前缀逐层定位子树，写锁粒度降到子树级别。收益：**写多场景和 key 集合持续增长（append-only）的场景不再退化**，整体内存更平稳。对外 API 和语义不变，「什么场景该用」的结论也不变。

### 7.3 适用场景（面试标准答案）

两个官方推荐场景，背下来：

1. **key 写一次、读多次，key 集合基本不变**（缓存类：连接池按 host 索引、路由表、单例注册表）；
2. **多个 goroutine 读写、覆盖不同的 key**（分片状写入，key 之间不相干）。

反场景（用普通 map + Mutex 更好）：**持续写同一批 key、写读混合、需要 Range 快照语义精确一致、需要 len()**——sync.Map 没有 len()（Go 1.24 之前没有，之后依然没有对外 len），Range 是最终一致遍历。数字记忆：写多读少时 sync.Map 可能比 mutex+map **慢数倍**。

---

## 8. 选型决策表

| 需求 | 用什么 | 备注 |
|---|---|---|
| 保护一个计数器/开关/配置快照 | atomic | 一个字以内；配置用 atomic.Pointer[T] 整体换 |
| 保护一段临界区（多个字/复合不变量） | Mutex | 默认选项，别过早优化 |
| 读临界区重（微秒级）+ 读写比高 | RWMutex | 先 benchmark；读里确认真只读 |
| 一批子任务等齐 / 带错误收集 | errgroup > WaitGroup | errgroup 顺便解决取消传播 |
| 高频短命对象减 GC | sync.Pool | Put 前 Reset；不是缓存不是连接池 |
| 读多写少共享 map | sync.Map | key 稳定 / 分片写入两个场景 |
| 跨 goroutine 传数据/所有权/事件 | channel | 见 `06`/`13`；锁保护状态，channel 传递数据 |

最后的选型元规则：**锁保护的是不变量，不是代码行**。先写出「任何时候必须为真的约束」（如 `m 里的 item 和磁盘上的文件必须一致`），每条不变量对应一把锁、明确谁加谁解，比背十种锁的 API 更能避免死锁。

死锁排查速记：`go test -race` 抓 race；死锁时 `kill -QUIT`（SIGQUIT）dump 全部 goroutine 栈，卡在 `semacquire` 的链条就是等待链；`GODEBUG=schedtrace=1000` 看调度概览。

---

## 9. 面试高频

**Q1：Mutex 是公平锁吗？**
非绝对公平。默认正常模式：被唤醒的 waiter 和新来的竞争，新来的（已在 CPU 上）常赢，吞吐高但可能饿死 waiter。waiter 等待超 1ms 切饥饿模式：锁直接交接队头，新来者不自旋不抢。拿到锁时若等待不足 1ms 或是队尾，切回正常。双模式是吞吐与公平的折中。

**Q2：Mutex 正常/饥饿模式切换的条件？**
见 Q1：>1ms 进饥饿，<1ms 或队尾出饥饿。饥饿模式还禁用自旋。

**Q3：为什么 Unlock 可以由非加锁者调用？**
Mutex 不记属主，Unlock 只是清 locked 位 + 发信号量。合法但不推荐——错误率指数上升，review 打回。反过来「未锁定就 Unlock」直接 fatal panic，不可 recover。

**Q4：自旋的条件？**
多核 && GOMAXPROCS>1 && 本 P 本地队列为空 && 自旋 <4 次 && 非饥饿模式。本质是赌持锁者马上释放，用几十 ns 空转换微秒级的调度唤醒成本。

**Q5：RWMutex 写锁等待时新读者会怎样？被阻塞。**
写者 Lock 时把 readerCount 减一个大常数打成负数，之后所有 RLock 看到负数就睡 readerSem。所以写者不会饿死。推论：读锁不能升级（持读锁再 Lock 死锁），能降级（Lock→RLock→RUnlock→Unlock）。

**Q6：RWMutex 一定比 Mutex 快吗？**
不一定。RWMutex 单次操作成本更高（多两个信号量 + atomic 计数），读临界区极短（百 ns 级）时是负优化。读临界区微秒级 + 读写比 >10:1 才明显赚。先 benchmark。

**Q7：WaitGroup 的 Add 为什么要在 goroutine 外调用？**
Wait 返回发生在计数归零时；若在子 goroutine 里才 Add，主流程可能已经 Wait 返回甚至重用 wg，触发 misuse panic（Add 与 Wait 并发）。协议：先 Add 后 go，子 goroutine 里 defer Done。

**Q8：atomic 能保护两个变量的一致性吗？**
不能。两个 atomic 字之间没有原子性，读者能观察到中间态。合成一个字（位打包/atomic.Pointer 换整个结构体快照）或上锁。另记：没有原子浮点 Add；atomic.Value 必须同类型，泛型时代用 atomic.Pointer[T]。

**Q9：sync.Pool 什么时候清空？放回前要做什么？**
每次 GC 周期清理（Go 1.13+ 主/victim 两级，对象活两个 GC 周期）。放回前必须 Reset 清状态，否则下个使用者拿到脏数据。定位：减 GC 压力，不是缓存/连接池/free list。

**Q10：sync.Map 的适用场景？**
① key 写一次读多次且集合稳定（注册表/路由）；② 多 goroutine 写不相干的 key。内部：Go 1.23- 是 read/dirty 双 map + misses 晋升，Go 1.24 起换 HashTrieMap（写锁子树级）。写多场景用 map+Mutex 更快；没有 len()。

**Q11：锁和 channel 怎么选？**
锁保护**共享状态的不变量**（「任何时候 m 与磁盘一致」）；channel **传递数据/所有权/事件**（「这个数据现在归你」）。官方口诀：share memory by communicating。混用判断：如果 goroutine 之间是「围绕一份数据反复读写」，锁；如果是「流水线生产-消费」，channel。

---

本篇对应实验：experiments/07_sync_atomic.go
