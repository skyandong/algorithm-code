# context 与错误处理

> **核心认知：** context 把「取消、超时、截止、请求级元数据」标准化成一棵树，父节点取消会沿树**广播**给所有后代，阻塞中的 goroutine 因此能被及时释放。错误处理对应的一句话是：**error 是值，比较的是链不是字符串**——`%w` 成链，`errors.Is/As` 沿链查。

本文基于 Go 1.26 的实际行为编写。前置知识：channel 关闭广播语义见 `05-channel内部与nil语义.md`，goroutine 泄漏见 `05-channel内部与nil语义.md` 第 13 节。

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

---

## 1. context 的核心模型：一棵取消树

```go
// src/context/context.go（节选）
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

reqCtx 超时（或客户端断开）→ Done channel 关闭 → dbCtx/rpcCtx/workerCtx 的 Done **同一时间**全部关闭。子节点的截止时间**只能比父更近**：`WithTimeout(parent, 10s)` 而父还剩 3s 时，子实际 3s 后取消。

两个易错点：

1. **Done() 是关闭 channel，不是发送值**。所以监听端标准写法永远是 `select { case <-ctx.Done(): return ctx.Err() ... }`——关闭对所有接收者同时可见（广播），这正是 `05` 篇「closed channel 永远可读」语义的应用；
2. **ctx 取消不等于 goroutine 自动停止**。取消只是「信号已拉响」：那些正阻塞在 `ctx.Done()`、`select`，或者阻塞在支持 ctx 的 IO（net/http、database/sql、grpc 调用）上的代码会被唤醒；**而正在跑纯 CPU 循环的代码，根本不知道取消已经发生**。context 是协作式取消，每个长循环都有义务主动检查。

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

### 1.1 根节点：Background 与 TODO

```go
// src/context/context.go（节选）
type emptyCtx struct{}                                     // 零大小，不分配堆内存

func (emptyCtx) Deadline() (deadline time.Time, ok bool) { return } // 零值时间 + false
func (emptyCtx) Done() <-chan struct{}                     { return nil }  // ★ nil，不是已关闭的 channel
func (emptyCtx) Err() error                                { return nil }
func (emptyCtx) Value(key any) any                         { return nil }

type backgroundCtx struct{ emptyCtx }
func (backgroundCtx) String() string { return "context.Background" }

type todoCtx struct{ emptyCtx }
func (todoCtx) String() string { return "context.TODO" }
```

四个方法全是空壳，结论就这四条：**永不取消、没有截止、没有值、`Err()` 永远为 nil**。

`Background` 与 `TODO` 的实现**只差一个字节**——`String()` 的输出。行为完全一致，差别是**写给人看的意图**：

| | 含义 | 用在哪 |
|---|---|---|
| `Background()` | **主动选择「我就是根」**：没有更上游的 ctx | `main`、`init`、测试、请求入口（HTTP / RPC handler 的顶层） |
| `TODO()` | **还没想好 / 暂时拿不到**：占位符，等着被真正的 ctx 替换 | 函数签名还没加 ctx 参数时的过渡期 |

四个容易答错的点：

1. **`Done()` 返回 nil channel，不是已关闭的 channel**——这个区别决定语义。已关闭的 channel 会让所有 `<-ctx.Done()` 立刻返回（等于「已取消」），而 **nil channel 上的接收永远阻塞**（等于「永不取消」），`select` 里那个 case 相当于被永久禁用。所以「Background 永不取消」不是一条口头约定，是 nil channel 的语义直接实现的；
2. **它不是单例**：`Background()` 每次返回一个新的零大小值（零大小不占堆内存，可放心在热路径调用）；
3. **`Value()` 永远返回 nil**：`Background` 本身挂不了任何东西，要挂就 `WithValue(Background(), k, v)` 派生一个；
4. **`WithoutCancel` 的 `Done()` 同样是 nil**，所以它和 `Background` 在「永不被取消」这一点上等价，区别是它还留着父链上的 Value。

**最大的坑：中途拿 Background 换掉上游传下来的 ctx，等于切断取消链。**

```go
func handler(req *http.Request) {
	ctx := req.Context()                      // 客户端断开 → 这个 ctx 会被取消
	go audit(context.Background())            // ✗ 断链：请求结束、上游超时，审计还在跑
	go audit(ctx)                             // ✓ 跟随请求生命周期，一起被取消
	go audit(context.WithoutCancel(ctx))      // ✓ 要收尾就显式剥离取消，Value 仍然保留
}
```

父取消后，用 `Background()` 起的那棵子树**一个信号都收不到**：上游已经超时、客户端已经断开，下游 goroutine 照样跑到底。这是 goroutine 泄漏最常见的来源之一。**不知道传什么就用 `TODO()` 占位**（它在提醒后来者「这里要补」），绝不要用 `Background()` 顶替一个已经存在的父 ctx。

顺带：**直接传 `nil` 会 panic**。`WithCancel(nil)`、`WithValue(nil, k, v)`、`WithoutCancel(nil)` 全都 panic `cannot create context from nil parent`；`WithValue` 还多两条——`nil key`（key 为 nil）和 `key is not comparable`（拿 slice、map 这类不可比较类型当 key）。所以「不知道传什么」也不能传 nil，`TODO()` 就是官方给出的答案。

---

## 2. 四种 With* 与一个 Value

| 构造函数 | 作用 | 必须 defer cancel？ |
|---|---|---|
| `WithCancel(parent)` | 手动取消（`cancel()` 关闭整棵子树） | 是（释放资源） |
| `WithTimeout(parent, d)` | 相对超时 | 是（同上，见 2.2） |
| `WithDeadline(parent, t)` | 绝对截止时刻 | 是 |
| `WithCancelCause(parent)` | 手动取消 + 附带**原因** | 是 |
| `WithTimeoutCause(p, d, c)` / `WithDeadlineCause(p, t, c)` | 超时 / 截止 + 附带原因 | 是 |
| `WithValue(parent, k, v)` | 挂请求级 KV | —（无资源） |
| `WithoutCancel(parent)` | 继承 Value 但**剥离取消** | — |
| `AfterFunc(ctx, f)` | ctx 取消时**另起一个 goroutine** 执行 f | 返回的 stop 按需调用 |

读取侧只有一个新函数：`context.Cause(ctx)` 取取消原因（没设过就回落到 `ctx.Err()`，未取消时返回 nil）。

### 2.1 取消原因：Cause 与 With*Cause

`Err()` 只有两个取值（`Canceled` / `DeadlineExceeded`），说不清「**为什么**被取消」。带 Cause 的这一组就是补这个洞：

```go
ctx, cancel := context.WithCancelCause(parent)
cancel(errUpstreamClosed)

ctx.Err()          // context.Canceled —— Err 仍然是那两个哨兵之一
context.Cause(ctx) // errUpstreamClosed —— 真正的原因
```

三条规则：

1. **`Err()` 不变，`Cause()` 补细节**：判超时、判取消照旧用 `errors.Is(err, context.DeadlineExceeded)`，要写日志、要定位「是谁取消的」才用 `Cause`；
2. **cause 沿树向下可见**：父设的 cause，在任意子 ctx 上调 `Cause` 都读得到（`Cause` 会顺着链找到内层的 `cancelCtx` 再取它的 `cause` 字段）；
3. **先取消的赢**：父带 `cause1`、子带 `cause2`，谁先被取消谁的原因生效——子先取消时 `Cause(子)` 是 `cause2`、`Cause(父)` 仍是 `cause1`；父先取消则两者都是 `cause1`。

`Cause` 在两处返回 nil：未取消的 ctx，以及 `WithoutCancel` 派生的 ctx——后者的定位本来就是跟取消这件事彻底脱钩。

### 2.2 为什么必须 defer cancel

**为什么 WithTimeout/WithCancel 必须 `defer cancel()`，哪怕函数马上返回？** 三条后果，按确定性排序：

1. **定时器要活到超时点**：`WithTimeout` 内部注册了一个 `time.AfterFunc` 定时器，不 `cancel()` 它就一直挂在 runtime 的定时器堆里，直到超时才停（`cancel()` 会立刻 `Stop` 它）。循环里不断派生 `WithTimeout(ctx, 1*time.Hour)` 却从不 cancel，等于在定时器堆里持续堆积；
2. **等在这个 ctx 上的 goroutine 不会自己醒**：下游若正阻塞在 `<-ctx.Done()`，只要没取消它就一直阻塞——这是 goroutine 泄漏最直接的来源；
3. **挂树的那一份要等父取消**：子 ctx 只有在父**可取消**时才会被挂进父的 `children` 集合（父的 `Done()` 为 nil 时压根不挂，见 4.2 节），挂上了就要等父取消或自己 `cancel()` 才摘除。

所以「父 ctx 活多久，泄漏的链就活多久」只对第 3 条成立——**父是 `Background` 时根本不挂树，代价落在前两条**。共同点是同一个：不 cancel，资源就一定要拖到超时点才释放。

```go
// 标准姿势
ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
defer cancel()
```

`WithoutCancel` 是高频新解法：**请求结束后仍要完成的收尾工作**（审计落库、发消息），用 `context.WithoutCancel(reqCtx)`——保留链路上的 trace/user 等 Value，但不再被请求取消连坐。之前只能重新 `context.Background()` 把 Value 丢光。

---

## 3. 传播规则与三大反面教材

规则四条（全部来自官方博客 "Go Concurrency Patterns: Context"）：

1. **ctx 作为第一个参数**，命名 `ctx`；
2. **不要把 ctx 存进结构体**——它是请求作用域的流水参数，存结构体会导致生命周期错配（例外：遵循「每个方法第一参数显式传 ctx」的大对象可以显式持有，如 http.Server 内部）；
3. **不要传 nil**——不知道传什么就 `context.TODO()`，`Background` 与 `TODO` 的取舍见 1.1 节（传 nil 会直接 panic）；
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

**反面教材三：忘记 cancel，资源拖到超时点才释放**（后果见 2.2 节）——循环派生 `WithTimeout(ctx, 1*time.Hour)` 却不 cancel，runtime 的定时器堆持续堆积；下游阻塞在 `Done()` 上的 goroutine 也一直不醒，压测长期跑能看到 goroutine 数与 RSS 缓慢上涨。

`propagateCancel` 里那句「起一个 goroutine 干等」只在**父节点是外部实现的 `Context`** 时才走（4.2 节第三条路），所以 pprof 里看到 `context.propagateCancel` 相关 goroutine 堆积，通常意味着链上混进了自定义 `Context` 实现，而不是单纯的忘记 cancel。

---

## 4. context 内部实现

面试能讲到这个深度就超过 90% 的候选人了。

### 4.1 cancelCtx：四个字段，一把锁

```go
// src/context/context.go（节选）
type cancelCtx struct {
	Context                          // 父节点（内嵌，因此 Deadline/Value 默认委托给父）

	mu       sync.Mutex            // 保护下面三个字段
	done     atomic.Value          // chan struct{}，惰性创建，cancel 时关闭
	children map[canceler]struct{} // 活着的子节点，cancel 后置为 nil
	err      atomic.Value          // 非 nil 即已取消
	cause    error                 // Cause 拿到的原因，cancel 时顺带写入
}
```

只有 `done` 和 `err` 用了 `atomic.Value`（`Err()` 是热路径，源码注释写着「原子读比加锁快约五倍」），**`children` 仍是一把 Mutex 保护的普通 map**，没有所谓无锁状态机。

`timerCtx` 与 `afterFuncCtx` 都是内嵌一个 `cancelCtx` 再加自己的字段：

```go
// src/context/context.go（节选）
type timerCtx struct {
	cancelCtx
	timer    *time.Timer // 受 cancelCtx.mu 保护
	deadline time.Time
}
```

`timerCtx.cancel` 的顺序是先委托 `cancelCtx.cancel` 广播完子树，再 `Stop` 定时器、再从父的 children 里摘除自己——**先广播，再收尾**。

还有一个零分配的细节：包级变量 `closedchan` 在 `init` 里就被关闭。若某个 ctx 从头到尾没人调过 `Done()`，cancel 时直接把 `closedchan` 存进 `done` 字段，连一个 channel 都不用造。

### 4.2 propagateCancel：挂树的三条路

每次 `With*` 都会走一遍它，决定「父被取消时怎么通知到我」：

```go
// src/context/context.go（简化）
func (c *cancelCtx) propagateCancel(parent Context, child canceler) {
	c.Context = parent

	done := parent.Done()
	if done == nil {
		return // ★ 父永不取消（Background / TODO / WithoutCancel）：不挂树，也不起 goroutine
	}
	select {
	case <-done: // 父已经取消了
		child.cancel(false, parent.Err(), Cause(parent))
		return
	default:
	}

	if p, ok := parentCancelCtx(parent); ok { // 找到最近的内层 *cancelCtx
		p.mu.Lock()
		if err := p.err.Load(); err != nil {
			child.cancel(false, err.(error), p.cause)
		} else {
			if p.children == nil {
				p.children = make(map[canceler]struct{})
			}
			p.children[child] = struct{}{} // 主流路径：挂进父的 children
		}
		p.mu.Unlock()
		return
	}
	// 父是外部实现的 Context（Done 是别人的 channel）：只能起一个 goroutine 干等
	go func() {
		select {
		case <-parent.Done():
			child.cancel(false, parent.Err(), Cause(parent))
		case <-child.Done():
		}
	}()
}
```

三条路：**能挂就挂进父的 `children`**、**父支持 `AfterFunc` 就注册回调**、**都不行才退化成起一个 goroutine 干等**。开头那个 `done == nil` 的短路，正是 `Background` 作根时零开销的原因——它连挂树的动作都不做，所以「以 Background 为父却忘记 cancel」并不会让子 ctx 一直挂在根上（2.2 节第 3 条就是这个意思）。

`parentCancelCtx` 找父的那一下有点巧：它不做类型断言，而是用包内私有 key 调 `parent.Value(&cancelCtxKey)`，沿链问出最近的内层 `*cancelCtx`——**所以中间夹多少层 `WithValue` 都不影响挂树**；问到之后还要比对 `p.done` 与 `parent.Done()` 是不是同一个 channel，不一致说明链上有人自定义了 Done，那就不能绕过它。

### 4.3 cancel：关、传、摘，一次持锁完成

```go
// src/context/context.go（简化）
func (c *cancelCtx) cancel(removeFromParent bool, err, cause error) {
	c.mu.Lock()
	if c.err.Load() != nil {
		c.mu.Unlock()
		return // 已取消：幂等
	}
	c.err.Store(err)
	c.cause = cause
	if d, _ := c.done.Load().(chan struct{}); d == nil {
		c.done.Store(closedchan) // 没人监听过：复用全局已关闭 channel
	} else {
		close(d)
	}
	for child := range c.children {
		child.cancel(false, err, cause) // 持父锁递归（源码注释明确标注了这个取舍）
	}
	c.children = nil
	c.mu.Unlock()

	if removeFromParent {
		removeChild(c.Context, c) // 把自己从父的 children 里摘掉
	}
}
```

四条要点：**幂等**（重复 `cancel()` 无副作用，所以 `defer cancel()` 可以随手写）、**关闭 done** 一次性广播给所有监听者、**递归 cancel 整棵子树**后清空 children、最后**把自己从父节点摘除**。整体 O(子树大小) 一次完成；之后才派生的子节点，会在 `propagateCancel` 开头发现父已取消而立刻自我了断。

### 4.4 Err() 的保证：Err 非 nil 时 Done 一定已关闭

```go
// src/context/context.go（节选）
func (c *cancelCtx) Err() error {
	if err := c.err.Load(); err != nil {
		<-c.Done() // ★ 先确保 done 已经关闭，再返回错误
		return err.(error)
	}
	return nil
}
```

那行 `<-c.Done()` 是在补一个 happens-before：并发下存在「err 已写入但 done 还没关闭」的瞬间，此时直接返回会让调用者看到 `Err() != nil` 却读不到 Done 关闭。`Err()` 宁可多一次接收也要保证——**`Err() != nil` 蕴含 `Done()` 已关闭**，这正是「先判 `Err()` 再 `<-Done()`」这类写法不会漏信号的根据。

### 4.5 Value 是一条链，查找是 O(链长)

`valueCtx` 就是带一对 key/val 的链表节点，每调一次 `WithValue` 就多一层：

```go
// src/context/context.go（节选）
type valueCtx struct {
	Context
	key, val any
}
```

`Value` 是一层一层往外问的（源码的 `value()` 对 `*valueCtx`、`*cancelCtx`、`*timerCtx`、`withoutCancelCtx` 分派，遇到 `backgroundCtx` / `todoCtx` 直接返回 nil）。两个推论：**查找成本随链长线性增长**，几百层的 Value 链不是免费的；**同一个 key 后挂的先被命中**（查找从最内层往外走，撞车时是内层悄悄覆盖外层）——这就是 key 必须用私有类型的真正原因。

最后一句提醒：这一节讲的都是**实现**，不是契约。契约只有 `Context` 接口那四个方法。依赖「children 是 map」「cancel 持父锁递归」去写代码，实现一改就会跟着坏——与 `01` 篇对 map 内部结构的告诫是同一条。

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
err := fmt.Errorf("open db: %w", ErrConnRefused)
// err 的树：fmt.wrapError{msg, ErrConnRefused}
errors.Is(err, ErrConnRefused) // true —— 沿 Unwrap 链找 == 目标
```

`%v` 只是拼字符串（链断了），`%w` 是「保留可判定性」的包装。错误链可以是**树**不只是链：`fmt.Errorf` 支持多个 `%w`，`errors.Join(e1, e2)` 生成多叉节点，`errors.Is/As` 会遍历整棵树。

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

把循环体抽成函数，实际 Go 的 defer 是函数级作用域，正解是抽函数。

**陷阱四：nil 函数 defer**

`var f func(); defer f()` —— defer 执行时才调用 f，nil 函数调用 panic，且发生在 defer 展开期，recover 姿势不对就接不住。

**性能补丁**（面试加分）：开放编码 defer（open-coded defer，条件：非循环内、非动态嵌套）把 defer 成本降到 ~1ns，与手写调用序基本持平。「defer 很慢」是老黄历，可放心用于临界区收尾——但仍别在百万次循环里堆 defer。

## 实验

对应代码：[experiments/09_context_error_test.go](experiments/09_context_error_test.go)（实现与断言同文件，无打印）

```bash
cd notes/golang
go test ./experiments/          # 全部实验（约 4 秒）
```
