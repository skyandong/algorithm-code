# algorithm-code

个人 Go 学习与面试备战仓库。

[![CI](https://github.com/skyandong/algorithm-code/actions/workflows/ci.yml/badge.svg)](https://github.com/skyandong/algorithm-code/actions/workflows/ci.yml)

> **找答案先查 [INDEX.md](INDEX.md)** —— 118 篇笔记、约 113 万字，按「面试官会怎么问」组织的横向入口。

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
│   ├── microservice/      # 14 篇：注册发现/配置中心/熔断限流/网关/可观测（07~14 为指标监控落地）+ monitoring/ 一键监控栈
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
    ├── pprof-lab/         # pprof 排障实战场（带病服务 + 手抓 profile 产物 + Web UI 工作流）
    └── trtc-demo/         # 腾讯云 TRTC
```

## 说明

- 主模块只包含 `algorithms/` 和 `engineering/`，其余目录均为独立 go module，单独构建
- 验证主模块用 `go vet ./...`（部分子目录包名与目录名冲突，`go build` 会误报）
- 笔记新增遵循体例：`NN-主题.md` 分册 + README 索引（含数据锚点表）+ experiments 实验
- 笔记体系的学习主线：golang（语言）→ design-pattern（范式）→ distributed（理论）→ system-design/microservice（应用设计）→ 各中间件（组件深度）→ interview（输出表达）

## CI

`.github/workflows/ci.yml` 在 push / PR 时自动发现所有 go module 并执行：

- `gofmt` 全仓格式检查
- `go vet` 遍历每个 module（新增 module 无需改配置）
- `go test` 只跑不依赖外部服务的部分：主模块全量 + `demos/trtc-demo` 的 `./sig/`

两点例外：

- `notes/mysql/ent` 是依赖未声明的孤儿历史代码，gofmt 与 vet 均跳过
- `notes/akafka` 的测试需要真实 Kafka broker，不纳入 CI。本地跑：`cd notes/akafka/docker && docker compose up -d`

本机没有 `go` 时，用仓库外的工具链：`export PATH=/Users/tal/sdk/go1.26.3/bin:$PATH`
