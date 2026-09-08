# DDD：领域驱动设计

> **核心认知：** DDD 不是设计模式的上级目录，而是回答一个模式回答不了的问题——**边界从哪来**。设计原则告诉你「什么样的结构是好的」（SOLID），设计模式告诉你「怎么写出这个结构」（GoF），DDD 告诉你「结构应该切在哪里」：先和业务专家对出一张通用语言地图，按限界上下文切开，再让代码里的类型直接说业务的话。Go 落地 DDD 恰好是顺风局：没有继承就不需要精巧的领域对象层次，**贫血模型的解药不是 class 而是行为挂在值和结构体上 + 消费者侧仓储接口**，这两样前 01/05 篇全铺好了。
> **判断标准不变：** 加一个需求改几处。DDD 的贡献是把「改哪」从架构师脑子里搬到目录结构里——上下文边界 = 改动隔离边界。

按 Go 1.26 语义说明。前置知识：消费者侧接口与依赖注入见 01 篇，中间件分层见 03 篇，微服务拆分视角见 `notes/microservice/01` 与 `notes/system-design/01`。

---

## 目录

1. [DDD 解决什么问题](#1-ddd-解决什么问题)
2. [战略设计：通用语言与限界上下文](#2-战略设计通用语言与限界上下文)
3. [上下文映射与防腐层](#3-上下文映射与防腐层)
4. [战术设计：实体与值对象](#4-战术设计实体与值对象)
5. [聚合与聚合根：一致性边界](#5-聚合与聚合根一致性边界)
6. [领域服务、应用服务与分层](#6-领域服务应用服务与分层)
7. [领域事件与仓储](#7-领域事件与仓储)
8. [Go 落地：目录结构与务实原则](#8-go-落地目录结构与务实原则)
9. [什么时候不用 DDD](#9-什么时候不用-ddd)
10. [面试高频](#10-面试高频)

---

## 1. DDD 解决什么问题

先看病灶。贫血模型（Anemic Model）是 Java 三层架构的 Go 翻版，也是最常见的腐化起点：

```go
// 贫血：实体只有字段没有行为，业务逻辑全挤在 service
type Order struct {
    ID     string
    Status string   // 用 string 表达状态，裸奔
    Items  []OrderItem
    Total  float64  // 谁都能改
}

func (s *OrderService) Pay(o *Order) error {   // 业务规则散落在 service
    if o.Status != "pending" { return errors.New("状态不对") } // 规则一
    o.Total = calcTotal(o.Items)                // 规则二：总额要和明细一致？
    o.Status = "paid"
    return nil
}

func (s *RefundService) Refund(o *Order) error { // 另一个 service 又改一遍状态
    if o.Status != "paid" { ... }                // 规则一抄了一份，开始漂移
    o.Status = "refunded"
    return nil
}
```

三个症状，一个根源：

1. **不变量没人守**：「已支付才能退款」「总额恒等于明细之和」是业务的铁律，现在靠每个 service 自觉检查——`Total` 是裸字段，任何一行代码都能改坏它；
2. **通用语言断了**：代码里是 `OrderService.Pay`，产品文档里叫「订单支付」，测试同学嘴上是「付款成功」——同一个概念三种说法，需求翻译必然损耗；
3. **边界靠感觉切**：下单、支付、对账都叫订单业务，塞一个 `order` 包，三年后谁都不敢动。

DDD 对症下药：战术设计（第 4~7 节）解决不变量——**状态和行为放在一起，字段私有，只能通过领域方法改**；战略设计（第 2~3 节）解决边界——**先切上下文再写代码**。

---

## 2. 战略设计：通用语言与限界上下文

战略设计是 DDD 的主菜，战术只是配菜。两件事：

**通用语言（Ubiquitous Language）**：业务专家、产品、开发用同一套词，且这些词**直接成为代码里的类型名和方法名**。订单上下文里没有「用户」只有「买家」，那么类型就叫 `Buyer`——客户上下文里的 `Customer` 不许混进来。语言对不上，说明边界切错了。

**限界上下文（Bounded Context）**：一个模型的适用范围。「商品」这个词在不同上下文里是不同的东西：

| 上下文 | 「商品」长什么样 | 关心的不变量 |
|---|---|---|
| 目录上下文 | 名称、详情页、图片 | 展示完整、搜索友好 |
| 交易上下文 | SKU、价格、库存扣减 | 不超卖、价格快照 |
| 物流上下文 | 重量、体积、仓址 | 可配送、包裹拆合 |

三个上下文里「商品」字段几乎不重叠，**强行用一个 `Product` 结构体统一它们，就是 Microservice 篇讲过的分布式单体在单体内的重演**。限界上下文是模型的边界，不是团队的边界——一个微服务通常对应一个上下文，但一个上下文可以先是一个包。

切分信号（面试可背）：同一个词在不同会议上含义不同；一段业务逻辑要同时改两个看似无关的功能；团队 A 的 PR 频繁碰团队 B 的包。

---

## 3. 上下文映射与防腐层

上下文之间要交换数据，映射关系（Context Map）里最值钱的一条：**防腐层（Anti-Corruption Layer, ACL）**。

```go
// 交易上下文依赖外部「会员系统」的 RPC，返回结构长这样
type memberRPCResp struct {
    UID    int64
    Level  int      // 1~7，外部黑盒
    VipExp int64    // 时间戳，语义含糊
}

// ✗ 腐化：外部模型直接渗透进领域代码
if order.User.Level >= 6 && order.User.VipExp > time.Now().Unix() { ... }
// 三个月后会员系统改字段，交易上下文全线搜捕 Level>=6

// ✓ 防腐层：在自己的上下文里说自己的话
type buyerRepo interface {                     // 01 篇：消费者侧接口
    ByID(ctx context.Context, id BuyerID) (*Buyer, error)
}

type memberRepoACL struct{ rpc memberClient }  // 适配器：只翻译不夹业务
func (a memberRepoACL) ByID(ctx context.Context, id BuyerID) (*Buyer, error) {
    r, err := a.rpc.Get(ctx, int64(id))
    if err != nil { return nil, err }
    return &Buyer{ID: id, Privileged: r.Level >= 6}, nil  // 外部黑盒 → 领域概念
}
```

防腐层和 03 篇适配器的区别只在**意图**：适配器翻译签名，防腐层翻译**模型**，把外部语义翻译成通用语言后才准进领域层。腐化是单向的——外部模型渗透一次，领域语言就脏一片。

---

## 4. 战术设计：实体与值对象

战术设计只有五个积木，先讲最容易被混用的两个。

**值对象（Value Object）**：只关心值是什么，不关心是哪一个。判据：两个实例所有属性相等即相等。推论——**不可变**（改值 = 换新值）、可以随意复制、适合做 map key。

```go
// Money 是值对象：字段全私有、不可变、有行为
type Money struct {
    amount   int64  // 最小货币单位（分），float 存钱是事故
    currency string
}

func NewMoney(amount int64, currency string) (Money, error) {
    if amount < 0 || currency == "" { return Money{}, ErrInvalidMoney }
    return Money{amount: amount, currency: currency}, nil
}

func (m Money) Add(o Money) (Money, error) {   // 运算返回新值，绝不改自己
    if m.currency != o.currency { return Money{}, ErrCurrencyMismatch }
    return Money{amount: m.amount + o.amount, currency: m.currency}, nil
}
```

**实体（Entity）**：关心是哪一个。生命周期里属性随便变，靠**唯一标识**认人。`Order` 改了三次状态还是同一单——这是实体；`Order` 里的收货地址换成 `Address` 值对象后，语义上「换了一个地址」而不是「地址变了」。

**一行判据（面试高频）**：需要跟踪生命周期用实体，只是一组属性的描述用值对象。价格从 10 改成 20，你说「价格变了」还是「换了一个价格」？前者实体，后者值对象。经验法则：领域里大部分该是值对象，全项目都是实体通常是建模偷懒。

Go 落地：值对象用小结构体 + 值接收者 + 返回新值；实体用指针接收者 + 私有字段 + 行为方法。**没有基类，没有 Comparable 接口**——值对象相等性就是一个 `Equal` 方法。

---

## 5. 聚合与聚合根：一致性边界

聚合（Aggregate）是一组必须**作为一个整体满足不变量**的对象；聚合根（Aggregate Root）是这个整体对外的唯一入口。作用和数据库的外键约束+事务边界同构，但把一致性从「DB 帮你兜底」升级为「类型系统帮你兜底」：

```go
type Order struct {           // 聚合根
    ID        OrderID
    buyerID   BuyerID        // 跨聚合只存 ID，不持有 Buyer 实体（第 3 节防腐成果）
    status    OrderStatus    // 值对象化的状态，String() 可读
    items     []OrderItem
    paid      Money
}

// 所有字段私有，外部想改状态只能走方法——不变量在方法里集中把守
func (o *Order) Pay(ctx context.Context, m Money, now time.Time) error {
    if o.status != StatusPending {                    // 不变量一：状态机合法转移
        return fmt.Errorf("%w: 当前 %s 不能支付", ErrIllegalTransition, o.status)
    }
    total, err := o.totalOf(o.items)                  // 不变量二：应付金额
    if err != nil { return err }
    if !total.Equal(m) {
        return fmt.Errorf("%w: 应付 %s 实付 %s", ErrAmountMismatch, total, m)
    }
    o.status = StatusPaid
    o.paid = m
    o.record(OrderPaid{OrderID: o.ID, Amount: m, At: now})   // 领域事件（第 7 节）
    return nil
}
```

三条铁律（面试可背）：

1. **外部只引用聚合根，不引用内部对象**：`OrderItem` 只能通过 `Order` 的方法改，不存在「Service 直接改第 2 行明细」——绕过根就绕过了不变量；
2. **跨聚合只传 ID**：`Order` 里存 `BuyerID` 而不是 `*Buyer`。聚合之间是弱引用，聚合内部才是强一致。把整个对象图挂在根下面，加载和并发都会死给你看；
3. **一个事务只改一个聚合**：要改多个？拆成多步 + 领域事件串联（第 7 节），或者想想是不是上下文没切开。这条直接对接 `notes/distributed/03` 的分布式事务——聚合内用本地事务，聚合间本来就该走 Saga。

聚合怎么切？**按不变量切，不按表切**。订单和订单明细是一个聚合（总额恒等）；订单和买家不是（各自独立变化）。切错的信号：为了保证一致性你开始锁两个聚合，或者聚合方法里一半参数是别的聚合的实体。

---

## 6. 领域服务、应用服务与分层

行为优先挂在实体和值对象上（充血）；**跨实体/跨聚合的编排**放不下时才升级为领域服务；应用服务只做薄薄一层协调：

```go
// 领域服务：跨聚合的纯业务规则，无 IO
type TransferService struct{}
func (TransferService) Transfer(from, to *Account, m Money) error {
    if err := from.Debit(m); err != nil { return err }  // 各自聚合守自己的不变量
    return to.Credit(m)
}

// 应用服务：用例编排——事务、取聚合、调领域逻辑、发事件。不写业务判断
func (s *OrderApp) PayOrder(ctx context.Context, cmd PayCmd) error {
    o, err := s.orders.ByID(ctx, cmd.OrderID)   // 仓储（第 7 节）
    if err != nil { return err }
    if err := o.Pay(ctx, cmd.Amount, s.clock.Now()); err != nil { return err }
    return s.orders.Save(ctx, o)                // 事务边界在这里
}
```

判断归属的口诀：**规则进实体，流程进应用服务，跨聚合规则进领域服务**。应用服务里出现 `if` 业务判断（金额、状态），就是领域逻辑漏层——这正是贫血模型回潮的路径。

分层从内到外（依赖只准向内）：

| 层 | 职责 | Go 对应 |
|---|---|---|
| 领域层 | 实体/值对象/聚合/领域服务/领域事件 | 纯 Go 包，**零外部依赖**（不 import sql/http/proto） |
| 应用层 | 用例编排、事务边界 | handler 之下的 usecase/service 包 |
| 基础设施层 | 仓储实现、RPC 客户端、MQ | 实现领域层定义的接口（依赖倒置落点） |
| 接口层 | HTTP/gRPC 编解码、鉴权 | `web/` 的 hertzserver、`rpc/` 的 grpcserver |

领域层零依赖是验收标准：`go list -deps` 里看不到任何驱动包，单测不连数据库。做不到，说明防腐失败。

---

## 7. 领域事件与仓储

**领域事件（Domain Event）**：领域里已经发生的事实的过去时表达（`OrderPaid`、`OrderCancelled`）。两个用途：

1. **解耦聚合间的一致性**：支付聚合只管自己付钱，发布 `OrderPaid`；库存、积分、通知各自订阅——支付聚合根本不知道它们存在（对比第 5 节铁律三）；
2. **保留业务语言**：审计日志记「OrderPaid at ...」而不是「UPDATE orders SET status=...」。

进程内同步分发就够了，上 MQ 是架构升级不是 DDD 要求（对齐 `notes/akafka/01` 的定位）：

```go
type OrderPaid struct { OrderID OrderID; Amount Money; At time.Time }

type EventPublisher interface { Publish(events ...any) }   // 消费者侧接口

func (o *Order) record(e any) { o.events = append(o.events, e) } // 先攒在聚合里
// 应用服务 Save 成功后统一 flush 到 publisher——「持久化成功才算发生」
```

事件先攒在聚合里、事务提交后才发布，是「领域事件必须可靠」的最小实现；要跨进程可靠，本地消息表见 `notes/distributed/03`。

**仓储（Repository）**：聚合的集合语义——「按 ID 拿一个聚合根、存回去」，在领域层定义接口、基础设施层给实现（依赖倒置的教科书落点，01 篇第 4 节的具象化）：

```go
// 领域层定义（消费者侧，单方法起步）
type OrderRepository interface {
    ByID(ctx context.Context, id OrderID) (*Order, error)
    Save(ctx context.Context, o *Order) error
}
// 基础设施层给 MySQL/内存/mock 实现——测试里 3 行 stub，01 篇说的「mock 免费」
```

仓储是**聚合的**容器不是表的 ORM：`OrderRepository` 返回完整聚合，不存在「单独查 OrderItem」这种口子——那是把聚合从中间撕开。

---

## 8. Go 落地：目录结构与务实原则

一种可直接抄的目录（按上下文分包，上下文内再分层）：

```
trade/                        // 交易上下文（一个包 = 一个限界上下文的最小形态）
├── order/                    // 领域层：零外部依赖
│   ├── order.go              //   聚合根 Order + 行为 + 不变量
│   ├── money.go              //   值对象
│   ├── events.go             //   领域事件定义
│   └── repository.go         //   仓储接口（消费者侧）
├── app/                      // 应用层：用例编排
│   └── pay_order.go
└── infra/                    // 基础设施层：实现接口
    ├── mysql_order_repo.go
    └── member_acl.go         // 防腐层
```

务实原则（没有这些就是给 CRUD 上刑）：

- **先通用语言后战术模式**：类型名说人话的价值大于任何模式。目录还没建，先把词统一；
- **模块从简单开始**：先一个包内分层（domain/ 子包起步），上下文边界稳了再拆微服务。DDD ≠ 微服务，单体内 DDD 是常态；
- **CRUD 子域不建模**：后台配置、字典表直接 database/sql 走起，DDD 火力只给核心域。区分核心域/支撑域/通用域本身就是战略设计的一部分；
- **函数式 option、中间件、消费者侧接口照常用**：DDD 不替代本仓前 8 篇，领域层的 `NewOrder` 该用 options 还是用 options（02 篇）。

---

## 9. 什么时候不用 DDD

DDD 有明确的适用边界（面试答这个比背概念加分）：

| 适合 | 不适合 |
|---|---|
| 业务规则复杂且多变（交易、风控、计费） | 纯 CRUD（后台管理、配置中心） |
| 领域专家存在且能对话（金融、物流、SaaS） | 技术驱动的基础设施（代理、存储引擎——那是 system-design 的地盘） |
| 系统要活 3 年以上，改动是主旋律 | 一次性活动页、验证性原型 |
| 多团队协作需要清晰边界 | 单人小工具（抽象成本 > 收益） |

对照组：本仓 `algorithms/` 是算法域，没有业务不变量，硬套 DDD 只会产生空壳聚合；而 `microservice` 的监控栈里「告警规则」有真实的领域逻辑（抑制、分组、for 抗抖），反而是个可以练手的领域。

**不用 DDD 时用什么？——Fowler《企业应用架构模式》（PoEAA）的三档组织方式**，DDD 只是其中最重的一档：

| 模式 | 逻辑放哪 | 适合 |
|---|---|---|
| 事务脚本 Transaction Script | 每个业务操作 = 一个函数，脚本式从头走到尾 | CRUD、规则简单且各操作独立 |
| 表模块 Table Module | 一个类对应一张表，围绕记录集操作 | 以表格为中心的报表/批处理（Go 里少见） |
| 领域模型 Domain Model | 协作对象图，行为挂对象上（= DDD 战术设计） | 规则复杂、不变量多且会打架 |

Gin/Hertz 这类无主张路由库上的标准落点就是**事务脚本**——俗称 handler-service-dao 三层（Spring 的 Service 大函数、Go 社区的三层、教科书的事务脚本是同一个东西的三个名字）：

```go
// service 层：一个用例一个函数，校验就写在脚本里，不藏
func (s *OrderService) CreateOrder(ctx context.Context, req CreateReq) (*Order, error) {
    if err := validate(req); err != nil { return nil, err }
    po := toPO(req)
    if err := s.dao.Insert(ctx, po); err != nil { return nil, err } // DAO 直通
    return toBO(po), nil
}
```

两个纠偏：**贫血在这个模式下不是罪**——贫血的骂名来自「拿事务脚本的架构干领域模型的活」，CRUD 没有不变量要守，贫血 + 事务脚本就是最优解；分层纪律照常保留——错误只处理一次、消费者侧接口做 mock、functional options，这些模式不挑档位（Kratos 的 biz/data 介于两档之间：比纯脚本多接口边界，比领域模型少聚合把守）。

**升级信号（脚本 → 领域模型的迁移判据）**：同一业务规则出现在 3 个以上 service 函数里（开始漂移）；状态字段长出 if-else 状态机；并发改同一行要靠「改前再查一遍」兜底。见到这些，才是 DDD 登场的时刻——反之，别提前还债。

---

## 10. 面试高频

**Q1：DDD 是什么？和设计模式什么关系？**
方法论 vs 解法目录。DDD 管两件事：战略上用通用语言+限界上下文划边界，战术上用实体/值对象/聚合守住不变量。设计模式（含 GoF）是 DDD 落地时借用的具体手段——比如防腐层用适配器实现，工厂创建聚合根。层次：设计原则（评判标准）→ 设计模式（解法）→ DDD（边界从业务来）。

**Q2：贫血模型为什么不好？**
实体只有 getter/setter，不变量散落在 N 个 service 里靠自觉维护，`Total` 这类字段任何代码都能改坏；通用语言也断了——业务规则读代码看不出来。解药是充血：字段私有 + 行为挂实体 + 方法守不变量。Go 里没有继承负担，充血成本比 Java 低得多。

**Q3：实体和值对象怎么区分？**
要不要身份。有唯一 ID、属性变了还是它 → 实体；属性全等即相等、不可变、改值换新对象 → 值对象。价格改 10→20 是「换一个价格」（值对象），订单改状态还是这单（实体）。经验：多数该是值对象，全是实体说明建模偷懒。

**Q4：聚合根是什么？怎么切聚合？**
一致性边界的唯一入口：内部对象只能通过根的方法修改，不变量（总额恒等、状态机合法）集中在根上把守。切分按不变量不按表：必须原子地满足同一组规则的放一个聚合；跨聚合只存 ID，一个事务只改一个聚合，聚合间一致性靠领域事件+Saga。

**Q5：领域事件解决什么？**
两件事：聚合间解耦（支付聚合发 OrderPaid，库存/积分各自订阅，互相不知道存在）+ 业务语言留痕（审计记「订单已支付」而不是 SQL）。最小实现：事件攒在聚合里，事务提交后由应用服务发布；跨进程可靠再加本地消息表/MQ。

**Q6：防腐层是什么？没听过但肯定见过？**
上下文之间翻译外部模型的隔离层：外部 RPC 的黑盒字段（Level>=6、VipExp）在边界处翻译成本上下文的概念（Privileged），外部模型改动只脏防腐层不脏领域。等价物：消费者侧适配器 + 防腐意图；网关做协议鉴权翻译、grpcclient 里的响应转换都是它。

**Q7：DDD 和微服务什么关系？**
战略设计的限界上下文是微服务拆分的**理论依据**：一个上下文一个服务，服务间用防腐层/事件通信。反过来不成立：DDD 不要求微服务，单体内按上下文分包同样成立。先按上下文分好包，拆服务时才有缝可切。

**Q8：什么项目不该用 DDD？**
纯 CRUD、没有领域专家可对话、生命周期短的原型——抽象成本大于收益。DDD 的火力只给核心域（业务规则复杂多变的部分），支撑域/通用域（配置、字典）直接 CRUD。答出「不是银弹」本身就是加分项。

---

本篇对应实验：experiments/09_ddd.go
