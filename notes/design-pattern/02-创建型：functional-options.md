# 创建型：functional options、Builder、Once 单例

> **核心认知：** Go 没有构造函数重载，一个类型只有一种 `New(...)` 签名——**参数膨胀**（10 个参数、一半要默认值）是必然遭遇的问题。三个惯用解法按规模递进：**配置结构体**（5 个以内直传）、**functional options**（可选参数多、需要校验和文档）、**Builder**（多步构建有依赖关系）。而 Java 的 singleton 在 Go 里的正解是「包级变量 + sync.Once」——包本身就是单例容器，不需要一个 singleton 类。判断标准：**调用方代码是否自文档**（`WithTimeout(3s)` 一眼懂，`New(s, 5, nil, true, 3)` 是天书）。

按 Go 1.26 语义说明。前置知识：sync.Once 双检查的内存语义见 `notes/golang/05-并发内存可见性与sync.Once.md`。

---

## 目录

1. [问题：Go 没有构造重载](#1-问题go-没有构造重载)
2. [阶梯一：配置结构体](#2-阶梯一配置结构体)
3. [阶梯二：functional options](#3-阶梯二functional-options)
4. [阶梯三：Builder](#4-阶梯三builder)
5. [sync.Once 单例：包即容器](#5-synconce-单例包即容器)
6. [选型决策](#6-选型决策)
7. [面试高频](#7-面试高频)

---

## 1. 问题：Go 没有构造重载

```go
// Java 可以：Server(int port)、Server(int port, int timeout)、Server(Config c) ...
// Go 只能有一个 NewServer。于是出现参数地狱——

func NewServer(addr string, port int, timeout time.Duration, maxConn int,
    tls bool, certPath string, debug bool, logger *Logger) *Server

NewServer("localhost", 8080, 3*time.Second, 100, true, "cert.pem", false, nil)
// 调用方：第 5 个参数 true 是什么来着？想用默认 timeout 还得知道默认值是多少
```

四类典型痛点：

1. **默认值**：Go 没有参数默认值，零值又不一定是想要的默认；
2. **可读性**：连续同类型参数（两个 int）位置传错编译器不报错；
3. **演进**：每加一个配置项都是 breaking change（所有调用点改签名）；
4. **校验**：参数间有约束（开了 tls 就必须有 certPath）时，校验代码和报错信息难以组织。

---

## 2. 阶梯一：配置结构体

最朴素的方案——把参数打包：

```go
type ServerConfig struct {
    Addr    string
    Port    int
    Timeout time.Duration
    MaxConn int
}

func NewServer(cfg ServerConfig) *Server

// 调用方带字段名，自文档
NewServer(ServerConfig{Addr: "localhost", Port: 8080})
```

优点：直观、加字段不破坏调用方（缺省字段用零值）。缺点：

1. **零值即默认**——但你想要的默认往往不是零值（Timeout 默认 30s，零值是 0）；要么导出 `DefaultServerConfig()` 让调用方覆写，要么 New 内部逐字段判零；
2. **判零有歧义**：调用方真的想要 `Timeout: 0`（永不超时）时，无法和「没填」区分；
3. **可变配置**：config 结构体一旦被并发读写（热更新场景），要么加锁要么整体换（对照 golang 07 篇 atomic.Pointer 快照）。

**适用**：参数 ≤ 5 个、零值默认可接受、构建一步完成。**过此线就该上 options。**

---

## 3. 阶梯二：functional options

标准库之外最高频的 Go 构造惯例（gRPC server、zap、kitex 全在用）：

```go
// Option = 「一个改配置的函数」
type Option func(*serverOptions)

type serverOptions struct {
    addr    string
    timeout time.Duration
    maxConn int
}

// 默认值在 defaults() 里集中定义
func defaultOptions() serverOptions {
    return serverOptions{addr: ":8080", timeout: 30 * time.Second, maxConn: 1000}
}

// 每个 Option 是一个导出的 WithXxx 函数（闭包捕获参数）
func WithTimeout(d time.Duration) Option {
    return func(o *serverOptions) { o.timeout = d }
}

func WithMaxConn(n int) Option {
    return func(o *serverOptions) { o.maxConn = n }
}

// 构造函数：变长 options，逐个应用到默认配置上
func NewServer(opts ...Option) *Server {
    o := defaultOptions()          // 1. 默认值
    for _, opt := range opts {     // 2. 逐个覆写
        opt(&o)
    }
    return &Server{opt: o}         // 3. 校验放这里，见下
}
```

调用方形态——**自文档、可任意顺序、可零个**：

```go
NewServer()                                    // 全默认
NewServer(WithTimeout(3 * time.Second))        // 覆写一个
NewServer(WithTimeout(3*time.Second), WithMaxConn(10))
```

四个进阶点（面试加分）：

1. **可校验的 Option**：返回 error——`type Option func(*opts) error`，构造循环里短路返回。适合「WithTLSCert(path) 但文件不存在」这类需要 IO 的校验；
2. **防止零值歧义**：内部字段用 `*time.Duration`/`*int`（nil=没填，非 nil=显式设置），把「没填」和「填了零」区分开；
3. **Option 可以公开成「配置快照」**：`Option` 既是构造参数，也是运行时热更新的载体（`srv.Apply(WithTimeout(5s))`）——一条通道两个用途，配置中心推送的本地落点；
4. **代价**：代码量是 config 结构体的 3 倍、多一层间接、新手要看懂闭包。**不要为 3 个参数上 options**（08 篇反模式）。

gRPC 源码里 `grpc.NewServer(grpc.MaxRecvMsgSize(...), grpc.Creds(...))` 就是这个模式的原型。

---

## 4. 阶梯三：Builder

当构建**分多步、步骤间有依赖**（先 SetHeader 再 SetBody 才能 Build），或产物是不可变对象（一次组装、终身只读）：

```go
b := strings.Builder{}   // 标准库自己就是示范
b.Grow(64)               // 可选的中间步骤
b.WriteString("hello")
s := b.String()          // 终态导出

// 自定义 builder：一个 SMTP 报文
type msgBuilder struct {
    msg Mail   // 私有，边攒边填
}

func (b *msgBuilder) From(addr string) *msgBuilder { b.msg.from = addr; return b } // 链式
func (b *msgBuilder) To(addrs ...string) *msgBuilder { ...; return b }
func (b *msgBuilder) Subject(s string) *msgBuilder   { ...; return b }
func (b *msgBuilder) Build() (Mail, error) {          // 终点：校验 + 固化
    if b.msg.from == "" {
        return Mail{}, errors.New("from is required")
    }
    return b.msg, nil
}

// 链式调用
mail, err := NewMsgBuilder().From("a@x.com").To("b@y.com").Subject("hi").Build()
```

和 options 的本质区别：**options 是一次性平面覆写（无序、独立），builder 是有序多步组装（步骤有依赖、最后统一校验固化）**。strings.Builder、bytes.Buffer、httptest 请求构造都是这个形态。

WHY `Build()` 返回 error 而 options 版可以不返回：builder 的校验依赖**多个步骤的组合结果**（From + To 都填了才能校验格式），只能在终点做；options 的每个 WithXxx 校验只看自己的参数，当场能做。

---

## 5. sync.Once 单例：包即容器

Java 的 singleton（私有构造 + getInstance + 双检查锁）在 Go 里**整个不需要**——包就是单例容器：

```go
// config/config.go —— 包级变量天然「进程内一份」
package config

var (
    once Once
    cfg  *Config
)

// Load 首次调用真正加载，之后所有调用直接返回缓存
func Get() *Config {
    once.Do(func() {
        cfg = loadFromFile("/etc/app.yaml") // 只会执行一次，无论多少 goroutine 并发进
    })
    return cfg
}
```

三个工程细节：

1. **为什么不用 init()**：init 在 main 之前跑，无法传参、无法失败重试、错误只能 panic。`sync.Once` 惰性初始化把「何时加载」交给第一次使用者，测试里还能注入（换掉 loadFromFile 的实现）；
2. **Once 的正确性**：golang 05 篇讲过——Once.Do 内部是 atomic 快路径 + mutex 慢路径 + 双检查，且「Do 返回时 f 一定已执行完」是 happens-before 保证，所有 goroutine 看到完整初始化的 cfg；
3. **何时真需要单例 vs 包变量**：无状态（logger 级别）→ 包变量直接赋值即可；初始化昂贵/依赖外部（配置、连接池）→ Once 惰性；需要按参数多份实例 → 那就不是单例，是工厂，别硬套。

**单例的代价照旧存在**：全局状态难测试（测试间泄漏）、隐藏依赖（谁用了 config 编译期看不出来）。Go 的缓解：包级变量至少让依赖在 import 图上可见，测试里用 `swap` 函数或接口注入替代。

---

## 6. 选型决策

| 场景 | 用什么 |
|---|---|
| 参数 ≤ 5、零值默认可接受 | 配置结构体 |
| 可选参数多、需要 WithXxx 自文档、要热更新通道 | functional options |
| 多步组装有依赖、产物不可变、终点校验 | Builder |
| 进程级一份 + 惰性初始化 | 包级变量 + sync.Once |
| 按参数产多份实例 | 工厂函数 `NewXxx(args)`（Go 的工厂就是普通构造函数） |

一个信号判断该升级了：**New 的调用点开始出现「为了跳过第 3 个参数而填零值」或「注释标着 // this true means xxx」**。

---

## 7. 面试高频

**Q1：functional options 是什么，解决什么问题？**
`NewXxx(opts ...Option)`，Option 是 `func(*options)` 闭包。解决 Go 无重载/无默认值下的参数膨胀：默认值集中定义、调用自文档（WithTimeout(3s)）、加配置不破坏调用方、可校验可组合。代价是代码量和间接层。gRPC/kitex 都在用。

**Q2：options 和 config 结构体怎么选？**
≤5 个参数、零值即默认 → config 结构体直接够。参数多/默认值非零/需要区分「没填」和「填了零」/需要热更新通道 → options（内部用指针字段区分）。

**Q3：什么时候用 Builder？**
构建是**多步且有序**（步骤依赖前步结果）、产物要不可变、校验依赖组合结果只能在终点做。options 是无序平面覆写，builder 是有序组装——这是本质区别。

**Q4：Go 怎么写单例？**
包级变量 + sync.Once 惰性初始化：包本身就是单例容器，不需要 singleton 类。不用 init() 是因为它无法传参/无法优雅失败/时机不可控。Once.Do 返回即完成初始化（happens-before），并发安全。单例的测试困难依然存在——依赖注入（构造函数收参数）仍是更可测的选择。

**Q5：Option 闭包里的校验失败怎么传出去？**
`type Option func(*opts) error`，构造循环短路返回第一个错误；或校验统一放 New 末尾。带 IO 的校验（读证书文件）必须用带 error 的版本。

---

本篇对应实验：experiments/02_creational.go
