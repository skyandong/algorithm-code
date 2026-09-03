# algorithm-code

个人 Go 学习与面试备战仓库。

## 目录分层

```
algorithm-code/
├── algorithms/        # 刷题区（主模块 github.com/skyandong/go-code）
│   ├── leetcode/          # 81 道题，16 分类，测试覆盖 99%
│   ├── leetcode-core/     # 21 道 S 级题 WHY 式深度注释重写版
│   ├── sort/              # 5 种排序算法
│   └── stack/             # 栈
├── engineering/       # 手写工程件：一致性哈希、环形计数器（带 design.md）
├── notes/             # 学习笔记（每栈独立 go.mod，编号分册 + experiments 可运行实验）
│   ├── golang/            # 13 篇：语言核心/并发/运行时/工程
│   ├── design-pattern/    # 8 篇：设计原则/创建/结构/行为/并发/错误/反模式
│   ├── distributed/       # 7 篇：CAP/Raft/事务/锁/分片/时钟/故障检测
│   ├── system-design/     # 8 篇：方法论估算/秒杀/短链/Feed/IM/分布式ID/延迟任务/缓存
│   ├── microservice/      # 6 篇：注册发现/配置中心/熔断限流/网关/可观测/服务网格
│   ├── mysql/             # 14 篇：索引/事务/锁/执行计划/分库分表/主从（ent/ 依赖未声明，见其 README）
│   ├── redis/             # 11 篇：底层/持久化/高可用/缓存三问/分布式锁/场景题
│   ├── akafka/            # 3 篇：架构/消息保障/原理与消费者
│   ├── elasticsearch/     # 检索
│   ├── tls/               # TLS 深入（自签证书产物给 nginx 用）
│   ├── nginx/             # 6 篇：架构/反代/限流/TLS 终止/调优/场景题
│   └── interview/         # 讲项目：STAR 模板 + 素材重组 + 追问预演
├── rpc/               # RPC 框架实验（各为独立 go module）
│   ├── grpcserver/        # gRPC 服务端：四种流 + 拦截器 + metadata/deadline + TLS
│   ├── grpcclient/        # gRPC 客户端：错误解包/流/metadata/deadline/TLS
│   └── kitexserver/       # CloudWeGo Kitex（thrift IDL）
├── web/               # Web 框架实验
│   ├── hertzserver/       # CloudWeGo Hertz
│   └── sse/               # Server-Sent Events（go-kratos）
├── ai/                # AI/LLM 实验
│   └── eino/              # CloudWeGo Eino：ADK 编排、RAG、向量检索、工具调用
└── demos/             # 其他独立 demo
    ├── http3/             # QUIC / HTTP3
    ├── oss-lab/           # 对象存储（aws-sdk-go-v2 S3 接口）
    ├── tracing/           # OpenTelemetry 链路追踪
    └── trtc-demo/         # 腾讯云 TRTC
```

## 说明

- 主模块只包含 `algorithms/` 和 `engineering/`，其余目录均为独立 go module，单独构建
- 验证主模块用 `go vet ./...`（部分子目录包名与目录名冲突，`go build` 会误报）
- 笔记新增遵循体例：`NN-主题.md` 分册 + README 索引（含数据锚点表）+ experiments 实验
- 笔记体系的学习主线：golang（语言）→ design-pattern（范式）→ distributed（理论）→ system-design/microservice（应用设计）→ 各中间件（组件深度）→ interview（输出表达）
