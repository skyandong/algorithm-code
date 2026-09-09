# 面试知识索引

按**面试官会怎么问**组织，而不是按文件罗列。所有路径相对仓库根目录。

笔记正文共 118 篇、约 113 万字，散在 12 个模块里。本文件是唯一的横向入口：想定位一个知识点，先来这里。

## 怎么用

- **考前突击**：直接翻第 10 节「速查表」，那是各模块浓缩好的一口答
- **按题找答案**：在下面 9 个域里找对应的问法 → 点进分册
- **想不起来叫什么**：用 `grep -rn "关键词" notes/` 全笔记搜

标注 `（高频）` 的是几乎必考、且答错会明显扣分的问题。

---

## 1. Go 语言核心

| 问题 | 位置 |
| --- | --- |
| slice 与 string 的头结构？append 扩容规则？ | [golang/01](notes/golang/01-slice与map底层.md) |
| slice 共享底层数组的三个经典坑 | [golang/01](notes/golang/01-slice与map底层.md) |
| map 底层实现（Swiss Table）？为什么无序？ | [golang/01](notes/golang/01-slice与map底层.md) |
| map 并发读写为什么 fatal error 且不可 recover（高频） | [golang/01](notes/golang/01-slice与map底层.md) |
| sync.Map 的 HashTrieMap 设计与选型 | [golang/01](notes/golang/01-slice与map底层.md)、[golang/06](notes/golang/06-sync锁与原子操作.md) |
| string 为什么不可变？与 []byte 转换的拷贝开销 | [golang/01](notes/golang/01-slice与map底层.md) |
| len("你好") 为什么是 6？rune 与 UTF-8 | [golang/01](notes/golang/01-slice与map底层.md) |
| iface 与 eface 的区别？接口装箱什么时候逃逸 | [golang/02](notes/golang/02-interface与反射.md) |
| nil 接口坑：为什么 `err != nil` 判不出（高频） | [golang/02](notes/golang/02-interface与反射.md) |
| 值接收者 vs 指针接收者：方法集差异 | [golang/02](notes/golang/02-interface与反射.md) |
| 泛型的实现：GC shape 分组 + 字典 | [golang/03](notes/golang/03-泛型.md) |
| 泛型 vs interface{} 怎么选 | [golang/03](notes/golang/03-泛型.md) |

## 2. Go 并发

| 问题 | 位置 |
| --- | --- |
| G / M / P 各自是什么？数量关系 | [golang/07](notes/golang/07-运行时调度器GMP.md) |
| 调度循环：runnext、本地队列、work-stealing | [golang/07](notes/golang/07-运行时调度器GMP.md) |
| 系统调用时 M 与 P 分离（hand off） | [golang/07](notes/golang/07-运行时调度器GMP.md) |
| GOMAXPROCS 在容器里的坑（高频） | [golang/07](notes/golang/07-运行时调度器GMP.md) |
| channel 的 hchan 结构 | [golang/05](notes/golang/05-Channel内部与nil语义.md) |
| nil channel 收发为什么永久阻塞？有什么用 | [golang/05](notes/golang/05-Channel内部与nil语义.md) |
| close 的底层语义与广播效应 | [golang/05](notes/golang/05-Channel内部与nil语义.md) |
| Mutex 的状态机与正常/饥饿两种模式 | [golang/06](notes/golang/06-sync锁与原子操作.md) |
| RWMutex 的真实成本，什么时候不该用 | [golang/06](notes/golang/06-sync锁与原子操作.md) |
| happens-before 与内存可见性（高频） | [golang/04](notes/golang/04-并发内存可见性与sync.Once.md) |
| 错误的双重检查锁定 vs sync.Once | [golang/04](notes/golang/04-并发内存可见性与sync.Once.md) |
| context 取消树的实现与传播规则 | [golang/09](notes/golang/09-context与错误处理.md) |
| Background vs TODO？子超时能比父长吗 | [golang/09](notes/golang/09-context与错误处理.md) |
| 并发模式：pipeline / fan-in / errgroup / semaphore | [design-pattern/06](notes/design-pattern/06-并发模式.md) |
| goroutine 裸奔、锁粒度错配等反模式 | [design-pattern/08](notes/design-pattern/08-反模式清单.md) |
| Goroutine 手写题与选择题 | [golang/11](notes/golang/11-Goroutine面试题集.md) |

## 3. Go 运行时与性能

| 问题 | 位置 |
| --- | --- |
| 逃逸分析：什么情况下栈变量跑到堆上 | [golang/08](notes/golang/08-内存管理与GC.md) |
| 分配器 mcache → mcentral → mheap（TCMalloc 思想） | [golang/08](notes/golang/08-内存管理与GC.md) |
| 三色标记 + 混合写屏障（高频） | [golang/08](notes/golang/08-内存管理与GC.md) |
| GOGC 与 GOMEMLIMIT 怎么配合 | [golang/08](notes/golang/08-内存管理与GC.md)、[golang/10](notes/golang/10-性能调优实战.md) |
| sync.Pool 每轮 GC 都清空，为什么还值得用 | [golang/08](notes/golang/08-内存管理与GC.md) |
| benchmark 规范与优化性价比排序 | [golang/10](notes/golang/10-性能调优实战.md) |
| pprof / trace 实战与线上排查标准路径（高频） | [golang/10](notes/golang/10-性能调优实战.md)、[demos/pprof-lab](demos/pprof-lab) |
| 火焰图怎么看：根在顶、宽度 ∝ 占比、正当热点与病灶同图 | [demos/pprof-lab](demos/pprof-lab) |
| Go 的正则会灾难性回溯吗（RE2 纠偏，高频陷阱题） | [golang/10](notes/golang/10-性能调优实战.md) 第 10 节、[demos/pprof-lab](demos/pprof-lab) |
| goroutine 泄漏怎么用 pprof 定位到具体行 | [demos/pprof-lab](demos/pprof-lab) |

## 4. MySQL

| 问题 | 位置 |
| --- | --- |
| 为什么用 B+ 树不用 B 树 / 哈希 | [mysql/01](notes/mysql/01-索引体系.md) |
| 聚簇索引 vs 非聚簇索引，回表是什么 | [mysql/01](notes/mysql/01-索引体系.md) |
| 最左前缀原则与索引失效场景（高频） | [mysql/01](notes/mysql/01-索引体系.md) |
| MVCC 原理、ReadView 生成时机 | [mysql/02](notes/mysql/02-事务与MVCC.md) |
| 快照读 vs 当前读，RR 怎么解决幻读（高频） | [mysql/02](notes/mysql/02-事务与MVCC.md) |
| 间隙锁与临键锁、RR 级加锁规则 | [mysql/03](notes/mysql/03-锁机制.md) |
| 死锁怎么产生、怎么排查 | [mysql/03](notes/mysql/03-锁机制.md) |
| redo / undo / binlog 各自的作用 | [mysql/05](notes/mysql/05-日志体系.md) |
| 两阶段提交为什么能保证 redo 与 binlog 一致（高频） | [mysql/05](notes/mysql/05-日志体系.md) |
| EXPLAIN 各字段怎么读，type 性能排序 | [mysql/04](notes/mysql/04-执行计划.md) |
| 优化器为什么有时不用索引 | [mysql/04](notes/mysql/04-执行计划.md) |
| 深分页怎么优化（LIMIT 1000000,10） | [mysql/14](notes/mysql/14-SQL优化实战.md) |
| 分库分表的分片策略与拆分后四大问题 | [mysql/11](notes/mysql/11-分库分表.md) |
| 主从延迟的原因与缓解 | [mysql/13](notes/mysql/13-主从复制与高可用.md) |
| 改进版 LRU 与 Buffer Pool 脏页刷新 | [mysql/12](notes/mysql/12-BufferPool与内存.md) |
| Online DDL 与 MDL 锁导致的线上事故 | [mysql/06](notes/mysql/06-Online-DDL.md) |
| JOIN 三种变体与驱动表怎么选 | [mysql/09](notes/mysql/09-JOIN原理与驱动表.md) |

## 5. Redis

| 问题 | 位置 |
| --- | --- |
| Redis 为什么这么快（高频） | [redis/01](notes/redis/01-为什么这么快.md)、[redis/10](notes/redis/10-其他追问.md) |
| 五种数据类型的底层结构（skiplist、ziplist 等） | [redis/02](notes/redis/02-数据类型与底层结构.md) |
| RDB vs AOF 的取舍与混合持久化 | [redis/03](notes/redis/03-持久化.md) |
| 缓存穿透 / 击穿 / 雪崩（高频） | [redis/05](notes/redis/05-缓存常见问题.md) |
| 缓存与数据库一致性怎么做（高频） | [redis/05](notes/redis/05-缓存常见问题.md)、[system-design/08](notes/system-design/08-缓存体系设计.md) |
| 过期 key 的删除机制：惰性 + 定期 | [redis/06](notes/redis/06-内存管理与淘汰.md) |
| 内存淘汰策略怎么选 | [redis/06](notes/redis/06-内存管理与淘汰.md) |
| 事务 / Lua / Pipeline 的区别与决策树 | [redis/07](notes/redis/07-事务-Lua-Pipeline.md) |
| 分布式锁：加锁为何要一条命令、释放为何要 Lua（高频） | [redis/08](notes/redis/08-分布式锁.md) |
| TTL 悖论与 watchdog 续期、可重入锁 | [redis/08](notes/redis/08-分布式锁.md) |
| Redlock 的争议（高频） | [redis/08](notes/redis/08-分布式锁.md)、[distributed/04](notes/distributed/04-锁与协调实践.md) |
| bigkey / hotkey 排查与渐进式删除 | [redis/09](notes/redis/09-生产排查与运维.md) |
| CPU 毛刺与内存暴涨的排查顺序（高频） | [redis/09](notes/redis/09-生产排查与运维.md) |
| 主从复制、哨兵、集群怎么选 | [redis/04](notes/redis/04-高可用.md) |
| 场景题：UV 统计、签到、附近的人、限流、发号器 | [redis/11](notes/redis/11-场景题专题.md) |

## 6. 分布式与微服务

| 问题 | 位置 |
| --- | --- |
| CAP 的准确表述与常见误解（高频） | [distributed/01](notes/distributed/01-CAP-BASE-PACELC.md) |
| PACELC：被忽略的 L | [distributed/01](notes/distributed/01-CAP-BASE-PACELC.md) |
| Raft 选主与日志复制（高频） | [distributed/02](notes/distributed/02-共识：Raft.md) |
| 2PC / TCC / Saga / 本地消息表怎么选（高频） | [distributed/03](notes/distributed/03-分布式事务.md) |
| etcd vs Redis vs ZK 的锁语义对比 | [distributed/04](notes/distributed/04-锁与协调实践.md) |
| 一致性哈希为什么只挪 1/N | [distributed/05](notes/distributed/05-复制与分片.md)、[engineering/consistenthash](engineering/consistenthash) |
| 物理时钟为什么不可信，逻辑时钟怎么补 | [distributed/06](notes/distributed/06-时钟与顺序.md) |
| 心跳超时与 φ-accrual 故障检测 | [distributed/07](notes/distributed/07-故障检测.md) |
| 服务注册发现与优雅上下线（发布事故根源） | [microservice/01](notes/microservice/01-注册与发现.md) |
| 配置中心热更新与灰度回滚 | [microservice/02](notes/microservice/02-配置中心.md) |
| 熔断三态机与限流算法四件套（高频） | [microservice/03](notes/microservice/03-熔断降级限流.md) |
| 网关 vs BFF vs LB，灰度路由怎么实现 | [microservice/04](notes/microservice/04-网关.md) |
| 三支柱、四个黄金指标、trace 透传 | [microservice/05](notes/microservice/05-可观测性.md) |
| sidecar 的 iptables 劫持与 SDK 模式 trade-off | [microservice/06](notes/microservice/06-服务网格intro.md) |
| 指标四类怎么选？Histogram 能聚合而 Summary 不能的根源 | [microservice/07](notes/microservice/07-指标体系与数据模型.md) |
| 高基数是什么？user_id 进 label 会怎样、怎么治 | [microservice/07](notes/microservice/07-指标体系与数据模型.md)、[microservice/12](notes/microservice/12-Go服务埋点实战.md) |
| Prometheus 为什么用 pull？TSDB 三层结构（高频） | [microservice/08](notes/microservice/08-Prometheus架构与抓取.md) |
| rate 和 irate 区别？Counter 重启归零怎么处理（高频） | [microservice/09](notes/microservice/09-PromQL实战.md) |
| 多实例全局 P99 怎么写 PromQL？手算 P99 | [microservice/09](notes/microservice/09-PromQL实战.md)、[microservice/07](notes/microservice/07-指标体系与数据模型.md) |
| 告警基于症状还是原因？for 抗抖、Alertmanager 分组抑制（高频） | [microservice/10](notes/microservice/10-告警与Alertmanager.md) |
| RED 看板怎么设计？Grafana 存数据吗 | [microservice/11](notes/microservice/11-Grafana与看板.md) |
| Go 服务怎么接监控？埋点为什么必须原子操作 | [microservice/12](notes/microservice/12-Go服务埋点实战.md) |
| Exporter 和直埋的区别？MySQL/Redis/Kafka 各看什么指标 | [microservice/13](notes/microservice/13-Exporter速查.md) |
| CPU 飙高 / P99 毛刺 / 内存上涨怎么查（资源为何放最后） | [microservice/14](notes/microservice/14-监控场景题与排障手册.md) |

## 7. 系统设计

| 问题 | 位置 |
| --- | --- |
| 4S 框架与必背估算数字（高频） | [system-design/01](notes/system-design/01-方法论与估算.md) |
| 秒杀：六层削峰与不超卖闭环（高频） | [system-design/02](notes/system-design/02-秒杀系统.md)、[redis/11](notes/redis/11-场景题专题.md) |
| 短链服务：发号器选型与 301/302 | [system-design/03](notes/system-design/03-短链服务.md) |
| Feed 流：推拉结合与大 V 扇出风暴 | [system-design/04](notes/system-design/04-Feed流.md) |
| IM 系统：长连接网关与可靠性铁三角 | [system-design/05](notes/system-design/05-IM系统.md) |
| 分布式 ID：号段双 buffer 与雪花 | [system-design/06](notes/system-design/06-分布式ID.md) |
| 延迟任务：时间轮原理（现场画图级） | [system-design/07](notes/system-design/07-延迟任务系统.md)、[engineering/ringcounter](engineering/ringcounter) |
| 三级缓存与热点 key（高频） | [system-design/08](notes/system-design/08-缓存体系设计.md) |

## 8. 网络与中间件

| 问题 | 位置 |
| --- | --- |
| nginx master-worker 与事件驱动为什么能扛几万连接 | [nginx/01](notes/nginx/01-核心架构与配置.md) |
| location 匹配优先级（必考） | [nginx/01](notes/nginx/01-核心架构与配置.md) |
| proxy_pass 的 4 种写法与踩坑 | [nginx/02](notes/nginx/02-反向代理与负载均衡.md) |
| 透传头：后端怎么感知原始协议 | [nginx/02](notes/nginx/02-反向代理与负载均衡.md) |
| 502 / 504 排障对照表（高频） | [nginx/02](notes/nginx/02-反向代理与负载均衡.md) |
| 限流三件套与热重载原理 | [nginx/03](notes/nginx/03-限流与安全.md)、[nginx/01](notes/nginx/01-核心架构与配置.md) |
| 有了 nginx 为什么还要 API 网关（高频） | [nginx/07](notes/nginx/07-最小API网关.md) |
| auth_request 鉴权与身份透传怎么落地 | [nginx/07](notes/nginx/07-最小API网关.md) |
| 网关能防止后端被绕过直连吗（陷阱题） | [nginx/07](notes/nginx/07-最小API网关.md) |
| 灰度发布：权重 / cookie / 百分比怎么选 | [nginx/06](notes/nginx/06-场景题专题.md)、[nginx/07](notes/nginx/07-最小API网关.md) |
| TLS 1.2 握手全流程（高频） | [tls/02](notes/tls/02-TLS1.2握手.md) |
| TLS 1.3 快在哪、安全在哪 | [tls/03](notes/tls/03-TLS1.3握手.md) |
| 0-RTT 的代价与重放风险 | [tls/04](notes/tls/04-会话恢复与0-RTT.md) |
| 证书信任链与吊销机制 | [tls/05](notes/tls/05-证书体系.md) |
| ECDHE / AEAD / HKDF 一次讲清 | [tls/06](notes/tls/06-密码学基础.md) |
| 降级攻击、侧信道与 MITM | [tls/07](notes/tls/07-攻击与防御.md) |
| mTLS 与握手排查三板斧 | [tls/08](notes/tls/08-mTLS与实战排查.md) |
| 倒排索引与 segment 不可变性 | [elasticsearch/01](notes/elasticsearch/01-核心概念与倒排索引.md) |
| ES 写入流程与 NRT、深分页 | [elasticsearch/04](notes/elasticsearch/04-分片与写入流程.md) |
| query vs filter 上下文、BM25 | [elasticsearch/03](notes/elasticsearch/03-查询DSL与相关性.md) |
| Kafka 怎么保证消息不丢 | [akafka/02](notes/akafka/02-消息保障.md) |
| 幂等、事务与消费端去重 | [akafka/02](notes/akafka/02-消息保障.md) |
| ISR / HW / LEO 与副本同步 | [akafka/03](notes/akafka/03-原理与消费者.md) |
| Rebalance 与消费者组 | [akafka/03](notes/akafka/03-原理与消费者.md) |
| Kafka 能保序吗？全局有序怎么做到（高频） | [akafka/04](notes/akafka/04-顺序性.md) |
| 存储结构与 offset 定位：稀疏索引为什么不用 B+ 树 | [akafka/05](notes/akafka/05-存储与读路径.md) |
| EOS 事务底层：epoch 僵尸写防护、LSO、control batch | [akafka/06](notes/akafka/06-事务底层.md) |
| 分区数怎么估？过多/过少的代价（高频） | [akafka/07](notes/akafka/07-分区与容量设计.md) |
| 分区只能增不能减 + 消息体/副本配置三件套 | [akafka/07](notes/akafka/07-分区与容量设计.md) |
| 分区倾斜与数据热点治理 | [akafka/07](notes/akafka/07-分区与容量设计.md) |
| 位移提交竞态与并发消费陷阱 | [akafka/08](notes/akafka/08-消费并发模型.md) |
| max.poll.* 三件套、静态成员资格与 Rebalance 优化 | [akafka/08](notes/akafka/08-消费并发模型.md) |
| 消费背压：跟不上生产者怎么办 | [akafka/08](notes/akafka/08-消费并发模型.md) |
| lag 告警怎么设、积压排查手册 | [akafka/09](notes/akafka/09-监控与运维.md) |
| Kafka / RocketMQ / RabbitMQ 怎么选（高频） | [akafka/10](notes/akafka/10-选型对比.md) |

## 9. 设计范式、工程与行为面

| 问题 | 位置 |
| --- | --- |
| SOLID 在 Go 里的真实形态、组合优于继承 | [design-pattern/01](notes/design-pattern/01-设计原则的Go式解读.md) |
| functional options 与 Builder 的选型 | [design-pattern/02](notes/design-pattern/02-创建型：functional-options.md) |
| 装饰器 vs 代理 vs 适配器 | [design-pattern/03](notes/design-pattern/03-结构型：装饰器与代理.md) |
| io.Reader 为什么伟大、单方法接口定律 | [design-pattern/05](notes/design-pattern/05-接口设计模式.md) |
| 错误三分法、包装语境、Is/As/Join（高频） | [golang/09](notes/golang/09-context与错误处理.md)、[design-pattern/07](notes/design-pattern/07-错误处理即模式.md) |
| panic / recover 与 defer 的四个陷阱 | [golang/09](notes/golang/09-context与错误处理.md) |
| 代码腐化的十二条路（反模式清单） | [design-pattern/08](notes/design-pattern/08-反模式清单.md) |
| DDD 是什么？和设计模式什么关系 | [design-pattern/09](notes/design-pattern/09-DDD领域驱动设计.md) |
| 贫血模型为什么不好？充血怎么落 | [design-pattern/09](notes/design-pattern/09-DDD领域驱动设计.md) |
| 实体 vs 值对象判据（高频） | [design-pattern/09](notes/design-pattern/09-DDD领域驱动设计.md) |
| 聚合根三铁律、聚合怎么切（高频） | [design-pattern/09](notes/design-pattern/09-DDD领域驱动设计.md) |
| 领域事件 / 防腐层各解决什么 | [design-pattern/09](notes/design-pattern/09-DDD领域驱动设计.md) |
| TDD 循环是什么？为什么测试先行 | [golang/13](notes/golang/13-TDD测试驱动开发.md) |
| 表驱动测试怎么写、错误用例怎么断言 | [golang/13](notes/golang/13-TDD测试驱动开发.md) |
| Go 里怎么做 mock？要上框架吗 | [golang/13](notes/golang/13-TDD测试驱动开发.md)、[design-pattern/01](notes/design-pattern/01-设计原则的Go式解读.md) |
| 集成测试依赖外部服务，CI 怎么处理 | [golang/13](notes/golang/13-TDD测试驱动开发.md)、[akafka/experiments](notes/akafka/experiments) |
| 什么时候不该 TDD？覆盖率多少合格 | [golang/13](notes/golang/13-TDD测试驱动开发.md) |
| CRUD 不该用 DDD，那用什么？——事务脚本（高频） | [design-pattern/09](notes/design-pattern/09-DDD领域驱动设计.md) |
| 六边形/洋葱/整洁架构是什么关系（高频） | [design-pattern/10](notes/design-pattern/10-架构风格.md) |
| CQRS 是什么、分几级落地、代价在哪 | [design-pattern/10](notes/design-pattern/10-架构风格.md)、[system-design/02](notes/system-design/02-秒杀系统.md) |
| 事件溯源优缺点、和审计日志的区别 | [design-pattern/10](notes/design-pattern/10-架构风格.md)、[akafka/05](notes/akafka/05-存储与读路径.md) |
| 把学习项目讲成工程能力：STAR 模板 | [interview/讲项目](notes/interview/讲项目.md) |
| 「这是学习项目吧？」怎么接 | [interview/讲项目](notes/interview/讲项目.md) |
| 60 秒自我介绍口述脚本与反问清单 | [interview/讲项目](notes/interview/讲项目.md) |

---

## 10. 速查表（考前 30 分钟翻这个）

各模块浓缩好的一口答，比正文快得多：

| 模块 | 速查表 |
| --- | --- |
| Go 语言与并发 | [golang/面试一口答](notes/golang/面试一口答.md) |
| MySQL | [mysql/面试一口答](notes/mysql/面试一口答.md) |
| Redis | [redis/面试一口答](notes/redis/面试一口答.md) |
| 分布式 | [distributed/面试一口答](notes/distributed/面试一口答.md) |
| 系统设计 | [system-design/面试一口答](notes/system-design/面试一口答.md) |
| 微服务 | [microservice/面试一口答](notes/microservice/面试一口答.md) |
| 设计模式 | [design-pattern/面试一口答](notes/design-pattern/面试一口答.md) |
| nginx | [nginx/面试一口答](notes/nginx/面试一口答.md) |

> elasticsearch、tls、akafka 三个模块暂无速查表，正文也不厚，直接看分册即可。

## 11. 手写代码与实验

**刷题区**（主模块，全部带测试，`go test ./...` 可跑）

- [algorithms/leetcode](algorithms/leetcode) — 81 道，16 分类
- [algorithms/leetcode-core](algorithms/leetcode-core) — 21 道 S 级题的 WHY 式重写版（讲思路时看这个）
- [algorithms/sort](algorithms/sort) — 5 种排序
- [engineering](engineering) — 一致性哈希、环形计数器，带 design.md

**可运行实验**（每栈独立 go module）

| 模块 | 实验目录 |
| --- | --- |
| Go 语言与并发 | [notes/golang/experiments](notes/golang/experiments) |
| 设计模式 | [notes/design-pattern/experiments](notes/design-pattern/experiments) |
| 分布式 | [notes/distributed/experiments](notes/distributed/experiments) |
| 系统设计 | [notes/system-design/experiments](notes/system-design/experiments) |
| 微服务 | [notes/microservice/experiments](notes/microservice/experiments) |
| MySQL | [notes/mysql/experiments](notes/mysql/experiments) |
| Redis | [notes/redis/experiments](notes/redis/experiments) |
| Kafka | [notes/akafka/experiments](notes/akafka/experiments) |
| ES | [notes/elasticsearch/experiments](notes/elasticsearch/experiments) |
| TLS | [notes/tls/experiments](notes/tls/experiments) |
| nginx | [notes/nginx/experiments](notes/nginx/experiments) |

> Kafka 实验需要真实 broker：`cd notes/akafka/docker && docker compose up -d`，否则测试会挂起到超时。

---

## 维护

新增分册后，把它的高频问法加进对应域的表格。索引的价值在于「问法 → 位置」，不在覆盖篇数。
