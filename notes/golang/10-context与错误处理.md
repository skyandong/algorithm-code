# Context 与错误处理

> **核心认知：** context 是 Go 把「**取消、超时、截止、请求级元数据**」标准化成一棵树的方案——每个 With* 派生一个子节点，父节点取消沿着树**广播**给所有后代，任何一处阻塞在 IO/Channel 上的 goroutine 都能被及时释放。错误处理的对应认知是：**error 是值，比较的是树不是字符串**——`%w` 包一层形成错误链，`errors.Is/As` 沿链查找，于是「哨兵错误判断」和「结构化取值」都不再依赖脆弱的错误消息文本。这两件事共同构成 Go 服务端代码的骨架：**所有函数第一个参数是 ctx，所有返回以 error 结尾**。

按 Go 1.26 语义说明，版本分界处单独标注。前置知识：channel 关闭广播语义见 `06-Channel内部与nil语义.md`，goroutine 泄漏见 `12`/`13`。

---

## 目录

1. [context 的核心模型：一棵取消树](#1-context-的核心模型一棵取消树)
2. [四种 With* 与一个 Value](#2-四种-with-与一个-value)
3. [传播规则与三大反面教材](#3-传播规则与三大反面教材)
4. [context 内部实现](#4-context-内部实现)
5. [错误处理：error 是值](#5-错误处理error-是值)
6. [错误链：Is / As / Join](#6-错误链is--as--join)
7. [panic 与 recover](#7-panic-与-recover)
8. [defer 的四个陷阱](#8-defer-的四个陷阱)
9. [面试高频](#9-面试高频)

---

## 1. context 的核心模型：一棵取消树

```go
type Context interface {
    Deadline() (deadline time.Time, ok bool) // 截止时刻
    Done() <-chan struct{}                    // 取消信号：channel 关闭（不是发值！）
    Err() error                               // 取消原因：Canceled / DeadlineExceeded
    Value(key any) any                        // 请求级元数据
}
```

模型只有一句话：**取消沿树向下传播，从根到叶单向流动**。

```text
ctx (Background)
 └─ reqCtx = WithTimeout(3s)          ← HTTP 请求入口
     ├─ dbCtx  = WithTimeout(200ms)   ← 每层操作可再收紧
     │    └─ stmtCtx
     └─ rpcCtx = WithCancel(reqCtx)   ← 也可以只挂靠不收紧
          └─ workerCtx
```

reqCtx 超时（或客户端断开）→ Done channel 关闭 → dbCtx/rpcCtx/workerCtx 的 Done **同一时间**全部关闭。子节点的截止**只能比父更紧**：`WithTimeout(parent, 10s)` 而父还剩 3s 时，子实际 3s 后取消。

两个易错点：

1. **Done() 是关闭 channel，不是发送值**。所以监听端标准写法永远是 `select { case <-ctx.Done(): return ctx.Err() ... }`——关闭对所有接收者同时可见（广播），这正是 `06` 篇「closed channel 永远可读」语义的应用；
2. **ctx 取消不等于 goroutine 自动停止**。取消只是「信号已拉响」，阻塞在 `ctx.Done()`、`select`、或支持 ctx 的 IO（net/http、database/sql、grpc 调用）上的代码会被唤醒；**正在跑纯 CPU 循环的代码根本不知道取消发生**。context 是协作式取消，每个长循环都有义务主动检查。

```go
for {
    select {
    case <-ctx.Done():
        return ctx.Err()      // 协作式：循环自己负责退出
    default:
    }
    processNext()             // 单次耗时长的话，中间也要能感知取消
}
```

---

## 2. 四种 With* 与一个 Value

| 构造函数 | 作用 | 必须 defer cancel？ |
|---|---|---|
| `WithCancel(parent)` | 手动取消（`cancel()` 关闭整棵子树） | 是（释放资源） |
| `WithTimeout(parent, d)` | 相对超时 | 是（同上，见下） |
| `WithDeadline(parent, t)` | 绝对截止时刻 | 是 |
| `WithValue(parent, k, v)` | 挂请求级 KV | —（无资源） |
| `WithoutCancel(parent)` | 继承 Value 但**剥离取消**（Go 1.21+） | — |
| `AfterFunc(ctx, f)`（Go 1.21+） | ctx 取消时异步执行 f | 返回 stop 函数 |

**为什么 WithTimeout/WithCancel 必须 `defer cancel()`，哪怕函数马上返回？** 两个原因：

1. **timer 泄漏**：WithTimeout 注册了一个 runtime timer，不 cancel 它要到超时才释放；
2. **整棵子树无法回收**：cancelCtx 会被挂在父节点的 children 集合里，直到取消或超时才摘除。调研/后台任务里「循环派生 ctx 但不 cancel」是 goroutine/内存缓慢增长的经典来源——父 ctx 活多久，泄露的链就活多久。

```go
// 标准姿势
ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
defer cancel()
```

`WithoutCancel` 是 Go 1.21 的高频新解法：**请求结束后仍要完成的收尾工作**（审计落库、发消息），用 `context.WithoutCancel(reqCtx)`——保留链路上的 trace/user 等 Value，但不再被请求取消连坐。之前只能重新 `context.Background()` 把 Value 丢光。

---

## 3. 传播规则与三大反面教材

规则四条（全部来自官方博客 "Go Concurrency Patterns: Context"）：

1. **ctx 作为第一个参数**，命名 `ctx`；
2. **不要把 ctx 存进结构体**——它是请求作用域的流水参数，存结构体会导致生命周期错配（例外：遵循「每个方法第一参数显式传 ctx」的大对象可以显式持有，如 http.Server 内部）；
3. **不要传 nil**——不知道传什么就 `context.TODO()`；`context.Background()` 是「主动选择无父节点」（main/init/测试入口），`TODO` 是「还没想好」（占位）；
4. **Value 只放请求级元数据**（trace ID、用户身份、鉴权 token），不放业务参数。

三大反面教材：

**反面教材一：ctx 藏在结构体里传递**

```go
type Service struct{ ctx context.Context } // ✗
func New(ctx context.Context) *Service { return &Service{ctx: ctx} }
func (s *Service) Do() { query(s.ctx) }    // Do 的调用者无法收紧/取消这个 ctx
```

调用方对 Do 内部的 ctx 完全失去控制——想给它加 200ms 超时？做不到。ctx 必须在每次调用的参数里流动。

**反面教材二：Value 当隐式参数通道**

```go
ctx = context.WithValue(ctx, "userID", 42) // ✗ 还用 string 做 key（可撞车）
// 三层以下某个函数里
uid := ctx.Value("userID").(int)           // 强转 panic + 隐式依赖 + 不可测试
```

Value 的正当用途是「**横切关注点**」——日志/追踪中间件注入 traceID，业务代码无感知地带上它。一旦业务逻辑依赖 Value 里的值，函数签名就撒谎了：看起来无依赖，实际读隐藏输入。key 必须用私有类型避免撞车：

```go
type ctxKey struct{}                      // 零大小私有类型，包外无法构造
var userKey ctxKey
ctx = context.WithValue(ctx, userKey, uid)
```

**反面教材三：忘记 cancel 导致子树滞留**（见上节）——压测工具长期跑会发现 RSS 缓慢上涨，pprof goroutine profile 里大量 `context.propagateCancel` 相关 goroutine/引用链。

---

## 4. context 内部实现

面试能讲到这个深度就超过 90% 的候选人了。

```go
// context 包内部（简化）
type cancelCtx struct {
    Context                          // 父节点
    mu       sync.Mutex
    done     atomic.Value          // chan struct{}，惰性创建
    children map[canceler]struct{} // 活着的子节点
    err      error                 // 非 nil 即已取消
}
```

**propagateCancel（挂树）**：With* 时，若父节点可取消，则把自己加进父的 children（父已取消则立刻取消自己）。为避免无限生长，父节点是 `background` 或 Value-only 链时跳过挂靠。

**cancel（广播）**：`cancel()` 做三件事——关 done channel、递归 cancel 所有 children、把自己从父节点摘除。全是 O(子树大小) 一次完成，之后新派生的子节点在挂树时发现父已取消，立即自我了断。

**done 惰性创建**：`Done()` 第一次被调用才真正创建 channel——大量从不监听取消的 ctx（只用来传 Value/超时由调用方用 Deadline 自己判断）就完全不用付 channel 的钱。

Go 1.21 起 context 包内部还做了无锁化优化（done/children 用 atomic 状态机），取消百万级子树的成本大幅降低——但对外语义没有任何变化，这正是不依赖实现细节的反面教材（对比 `01` 篇 map 内部结构的告诫）。

---

## 5. 错误处理：error 是值

### 5.1 两个宇宙惯例

```go
// 1. error 放最后一个返回值；nil 表示成功
func Parse(s string) (Item, error)

// 2. 错误要么处理，要么包装上抛，绝不吞掉、绝不 _ 掉
v, err := Parse(s)
if err != nil {
    return fmt.Errorf("parse config: %w", err) // %w 包装，保留链
}
```

`if err != nil { return err }` 本身不是坏味道——**吞错、二次报告、丢上下文才是**。错误处理三问：这个错我能不能恢复？不能恢复就带上「我在做什么」再上抛；上抛时用 `%w` 还是 `%v`？想让人能 `errors.Is/As` 就用 `%w`。

### 5.2 哨兵错误与自定义错误类型

```go
var ErrNotFound = errors.New("not found")   // 哨兵：可比较的单一实例

type ValidationError struct {
    Field string
    Msg   string
}
func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }
```

哨兵用 `==`/`errors.Is` 比较；自定义类型承载结构化信息用 `errors.As` 取出。**永远不要用字符串内容判断错误**——`strings.Contains(err.Error(), "timeout")` 在错误消息改版、本地化、中间层重写消息后集体失效，这是线上事故的高发写法。

---

## 6. 错误链：Is / As / Join

### 6.1 %w 与 Unwrap：链是怎么形成的

```go
// Go 1.13+
err := fmt.Errorf("open db: %w", ErrConnRefused)
// err 的树：fmt.wrapError{msg, ErrConnRefused}
errors.Is(err, ErrConnRefused) // true —— 沿 Unwrap 链找 == 目标
```

`%v` 只是拼字符串（链断了），`%w` 是「保留可判定性」的包装。错误链在 Go 1.20 后是**树**不只是链：`fmt.Errorf` 支持多个 `%w`，`errors.Join(e1, e2)` 生成多叉节点，`errors.Is/As` 会遍历整棵树。

### 6.2 errors.Is vs errors.As

| | 判断什么 | 用法 |
|---|---|---|
| `errors.Is(err, target)` | 链上是否有**相等**（==）的节点；或某节点实现了 `Is(target) bool` | 哨兵错误：`io.EOF`、`sql.ErrNoRows`、`context.DeadlineExceeded` |
| `errors.As(err, &target)` | 链上是否有**可赋值给 target 类型**的节点 | 结构化错误：`var ve *ValidationError; errors.As(err, &ve)` |

工程高频组合：

```go
switch {
case errors.Is(err, context.DeadlineExceeded): // 超时 → 可重试
    retry()
case errors.Is(err, context.Canceled):          // 主动取消 → 不重试，直接返回
    return err
}

var netErr net.Error
if errors.As(err, &netErr) && netErr.Timeout() { ... }
```

### 6.3 包设计的三层错误纪律

1. **底层包（库）**：定义自己的错误类型/哨兵，返回裸错误，**不要**包一层 `fmt.Errorf("xxx: %v")` 把类型信息抹掉；
2. **中间层**：`%w` 包装加语境（「处理订单第 3 步」），保留链；
3. **顶层（RPC/HTTP handler）**：`errors.Is/As` 判定后映射为对外的错误码/HTTP 状态；日志在此打全链（带 %w 链的 err.Error() 自带上下文）。

配套工程习惯：**错误只处理一次**——层层打日志再上抛是日志事故（同一错误重复 N 行）；顶层收口处（recovery 中间件、RPC 拦截器）打一次带堆栈的完整日志即可。

---

## 7. panic 与 recover

### 7.1 语义模型

panic = **不可恢复错误的控制流逃逸**：立即展开当前 goroutine 的调用栈，逐个执行 defer；栈顶展开完若无人 recover → 打印堆栈、进程退出（exit 2）。

recover = **唯一合法的拦截点**：返回正在展开的 panic 值并止住展开。两条铁律：

1. **recover 必须在 defer 的函数里直接调用**——嵌套函数里调用返回 nil、无效；
2. **recover 只救当前 goroutine**——子 goroutine 里 panic，父的 defer 再完美也拦不住，进程照样死。这就是「每个长生命周期 goroutine 入口都套一层 recover」的由来（HTTP server 内置了；自己 `go` 出去的要自己包）。

```go
func worker() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("worker panic: %v\n%s", r, debug.Stack()) // 带栈，且不再 re-panic
        }
    }()
    ...
}
```

### 7.2 什么时候 panic，什么时候 error

| panic | error |
|---|---|
| 程序员错误（不可能发生）：数组越界、nil 解引用、断言失败、不可达分支 | **可预期的失败**：IO 错误、输入不合法、依赖不可用 |
| 初始化致命错误：配置文件缺失、监听端口失败（main 里 panic 是可接受的） | 一切「调用方需要决定怎么处理」的结果 |

库代码对外的 API **永不 panic**（断言/越界防御 + 转成 error）；「panic 跨包边界」是最恶劣的接口之一。标准库反例：`regexp.MustCompile` 名字里的 Must 就是显式宣告「只用于启动期字面量」。

### 7.3 recover 后 re-panic 还是吞掉

原则：**能处理才 recover，不能处理就让它死**。吞 panic 的隐患是系统处于未知状态继续跑（锁没释放、map 写了一半）——比崩溃更危险。折中方案：recover → 打日志/上报 → 按场景选择：请求级 goroutine（HTTP handler）吞掉返回 500；进程级关键路径 re-panic 让守护进程重启。

---

## 8. defer 的四个陷阱

defer 在 return 赋值**之后**、函数真正返回**之前**执行，LIFO 多个 defer。所有陷阱都是这句话的推论：

**陷阱一：参数立即求值**

```go
i := 0
defer fmt.Println(i) // 打印 0（入 defer 链时 i 的值已拷贝）
i = 1
```

要延迟求值就包成闭包：`defer func() { fmt.Println(i) }()` 打印 1。

**陷阱二：命名返回值 + defer 可以改写返回值**

```go
func f() (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("panic: %v", r) // 改的就是返回值本身
        }
    }()
    ...
}
```

recover 转 error 的标准姿势就靠它；反过来，「defer 里无意改了命名返回值」也是隐蔽 bug 源。

**陷阱三：循环里 defer 累积**

```go
for _, f := range files {
    fh, err := os.Open(f)
    ...
    defer fh.Close() // ✗ 所有句柄到函数结束才关，句柄耗尽
}
```

把循环体抽成函数，或 Go 1.22+ 后写 `defer` 在块级作用域…实际 Go 的 defer 仍是函数级，正解是抽函数。

**陷阱四：nil 函数 defer**

`var f func(); defer f()` —— defer 执行时才调用 f，nil 函数调用 panic，且发生在 defer 展开期，recover 姿势不对就接不住。

**性能补丁**（面试加分）：Go 1.13 开放编码 defer（open-coded defer，条件：非循环内、非动态嵌套）把 defer 成本降到 ~1ns，与手写调用序基本持平。「defer 很慢」是老黄历，可放心用于临界区收尾——但仍别在百万次循环里堆 defer。

---

## 9. 面试高频

**Q1：context 取消的传播机制？**
With* 时子 ctx 挂进父的 children 集合；cancel() 关闭自身 done channel、递归 cancel 所有 children、从父节点摘除。Done() 返回的 channel 关闭（不是发值），对所有 select 监听者同时可见。父先取消则新子节点挂树时立即自取消。

**Q2：为什么 WithTimeout 后必须 defer cancel？**
两个资源：runtime timer 不 cancel 要到超时才释放；cancelCtx 挂在父 children 里，不摘除整条子树无法被 GC。高频派生不 cancel = 泄漏，goroutine/RSS 缓慢上涨。

**Q3：子 ctx 的超时能比父长吗？**
不能。Deadline 取更早者：WithTimeout(parent, 10s) 而父还剩 3s，子实际 3s 取消。Value 继承父全部；取消链单向（父到子），子取消不影响父。

**Q4：ctx.Done() 是发送还是关闭？为什么重要？**
关闭。关闭是广播——任意多个监听者同时解除阻塞且不消费数据；发送只能唤醒一个且可能丢。这正是 channel 关闭语义（06 篇）在标准库的最大应用。

**Q5：context.Value 的适用边界？**
只放请求级横切数据（traceID、用户身份），key 用私有类型；业务参数必须显式出现在函数签名。反面：三层以下偷偷读 Value、string key 撞车、强转 panic。

**Q6：context.Background() 和 context.TODO() 的区别？**
语义意图不同：Background = 主动选择「根/无父」（main、init、测试）；TODO = 占位「还没决定」（重构中途）。行为完全一致——区别是给人看的。

**Q7：取消 ctx 后，正在跑 CPU 密集循环的 goroutine 会停吗？**
不会。context 是协作式取消：只唤醒阻塞在 Done()/支持 ctx 的 IO 上的代码。CPU 循环必须自己周期性检查 Done()。这也是 Go 没有抢占式 kill goroutine 的原因（GMP 篇）。

**Q8：errors.Is 和 errors.As 的区别？**
Is 沿 Unwrap 树找「相等」的节点（哨兵：io.EOF、context.Canceled），或调用节点的 Is 方法；As 找「可赋值给目标类型」的节点，取结构化字段。共同前提：包装用 %w 而不是 %v（%v 断链）。Go 1.20+ 支持多 %w 与 errors.Join，判定遍历整棵树。

**Q9：为什么不能用 strings.Contains(err.Error(), "timeout") 判断超时？**
依赖错误文本——消息重排/本地化/中间层改写即失效，且拼写错误编译期发现不了。正解：errors.Is(err, context.DeadlineExceeded) 或 errors.As(err, &netErr)+netErr.Timeout()。

**Q10：子 goroutine panic，外层 defer recover 能救吗？**
不能。recover 只作用于当前 goroutine 的栈展开；子 goroutine panic 没人接住就是进程退出。所以每个自己 go 出去的长生命周期 goroutine 入口都要包 defer+recover（net/http 的 handler 已内置）。

**Q11：recover 的位置有什么限制？**
必须在 defer 的函数里直接调用。defer func(){ recover() } 有效；func(){ defer recover() } 无效（recover 不在 defer 体内直接调用，返回 nil）。同理 recover() 直接写在函数体里也无效。

**Q12：defer 的执行时机和参数求值？**
return 赋值之后、真正返回之前，LIFO。参数在入 defer 链时立即求值拷贝（陷阱一）；命名返回值可被 defer 修改（recover 转 error 的标准姿势）；循环内 defer 累积到函数级（抽函数解决）。

---

本篇对应实验：experiments/10_context_error.go
