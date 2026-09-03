# design-pattern 学习笔记

> 目标：以「Go 惯用形态」系统掌握设计模式——不是翻译 Java 23 种，而是回答「Go 里这个模式长什么样、标准库谁在用、什么时候不该用」。全部结论配可运行实验，**每个模式都能在仓库现有代码里指出实例**（rpc 的 interceptor、grpc 的 options、web 的中间件）。
> **主线认知：** Go 砍掉继承和构造重载后，模式落在**接口、函数值、组合、channel** 四样东西上。判断模式好坏的唯一标准：加一个需求，你要改 0 处（组合）、加 1 处（扩展点），还是改 N 处（散弹修改）。

## 目录

1. [设计原则的 Go 式解读](01-设计原则的Go式解读.md) — SOLID 落地形态、组合优于继承、接受接口返回结构体
2. [创建型：functional options](02-创建型：functional-options.md) — config 结构体/options/Builder 三阶梯、Once 单例
3. [结构型：装饰器与代理](03-结构型：装饰器与代理.md) — 嵌入 vs 持有包装、中间件链、适配器与编译断言
4. [行为型：策略观察者状态机](04-行为型：策略观察者状态机.md) — 策略三层、channel 观察者、转移表状态机、命令闭包
5. [接口设计模式](05-接口设计模式.md) — io.Reader 生态、单方法定律、消费者侧接口
6. [并发模式](06-并发模式.md) — pipeline、fan-out/in、errgroup、semaphore、生命周期三问
7. [错误处理即模式](07-错误处理即模式.md) — 三分法、语境公式、只处理一次、重试边界
8. [反模式清单](08-反模式清单.md) — 12 条腐化路径 + review 信号总表
9. [面试一口答](面试一口答.md) — 考前速刷：高频问题「张口就来」

## 重点回顾(自测)

- [ ] 组合优于继承：嵌入无虚函数、换行为 = 换内层实例、多态唯一来源是接口
- [ ] 接受接口返回结构体；接口在消费者侧定义（谁用谁定义，mock 免费）
- [ ] 创建型三阶梯的选择：≤5 参数 config、可选多 options、多步依赖 Builder
- [ ] functional options 四件套：默认值集中 / WithXxx 闭包 / 可校验 / 指针字段区分没填
- [ ] 单例 = 包级变量 + Once；为什么不用 init()（传参/失败/时机三不可）
- [ ] 装饰器：嵌入版自动转发（横切）、持有版精细控制；可叠罗汉、顺序即语义
- [ ] 中间件 `func(Handler) Handler`：Chain 倒序包裹、能在转发前拦截（对象版不能）
- [ ] 装饰器 vs 代理：代码相同、意图不同（加行为 vs 管访问）
- [ ] 适配器只翻译不夹业务 + `var _ Interface` 编译断言
- [ ] 策略三层：匿名 / 命名 / 函数类型当领域概念
- [ ] 观察者：channel 版的核心决策是满时策略（背压 vs 丢弃）；不可丢事件走 MQ
- [ ] 状态机：转移表集中规则、Next 纯函数、非法转移显式报错
- [ ] io.Reader 为什么伟大：单方法捕捉最小不变量 + 隐式实现 + 装饰器组合
- [ ] 接口 1 个方法是黄金 3 个是上限；error 本身就是单方法接口
- [ ] goroutine 启动三问：何时退出 / 谁 recover / 谁等
- [ ] pipeline 关闭规则：发送方关闭、每阶段关自己的输出、fan-in 等齐后关总输出
- [ ] errgroup 替代裸 WaitGroup 的三件事：错误收集 + 取消传播 + SetLimit
- [ ] 错误三分法：哨兵(Is) / 结构化(As) / 即时包装(读文本)
- [ ] 错误只处理一次：中间层只包不记、顶层唯一日志 + 对外语义
- [ ] Canceled 绝不重试（方向信号）；重试要幂等 + 上限 + 退避
- [ ] 反模式识别：I:Impl 同名 / GetInstance / common 上帝包 / any+type switch

## 跑实验

```bash
cd notes/design-pattern
go run ./experiments/ all          # 全部实验
go run ./experiments/ structural   # 单跑：03 篇
# 可用名: principles|creational|structural|behavioral|interface|concurrency|errors

# 并发实验必开 race
go run -race ./experiments/ concurrency
```

**文件说明**

| 文件 | 内容 |
|------|------|
| `experiments/01-07_*.go` | 每篇笔记对应的可运行验证（08 反模式是清单无实验） |
| `experiments/06_concurrency.go` | 含手写 mini errgroup（展示 x/sync/errgroup 的原理） |
| `go.mod` | 独立 module `adesignpattern`（零外部依赖） |

## 与其他模块的衔接

- `notes/golang/03` — interface 二元组/方法集（本模块 03 实验里 *T 方法集陷阱现场重演）
- `notes/golang/06` — channel 关闭广播（并发模式的取消原语）
- `notes/golang/10` — %w/Is/As 机制细节（07 篇的模式化上层）
- `rpc/grpcserver/interceptor.go` — UnaryInterceptor：中间件模式的 RPC 实例
- `rpc/grpcclient` — options 模式的消费方视角
- `notes/redis/08` — 分布式锁 + 幂等键：重试边界的生产版
- `demos/tracing` — 装饰器/中间件在可观测性上的落地（handler 包一层 span）
