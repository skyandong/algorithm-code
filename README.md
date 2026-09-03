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
├── notes/             # 学习笔记：golang / mysql / redis / elasticsearch / kafka / tls
│                      # 每栈独立 go.mod，编号分册 + experiments 可运行实验
├── rpc/               # RPC 框架实验（各为独立 go module）
│   ├── grpcserver/        # gRPC 服务端：unary + status.WithDetails 结构化错误
│   ├── grpcclient/        # gRPC 客户端：错误详情解包
│   └── kitexserver/       # CloudWeGo Kitex（thrift IDL）
├── web/               # Web 框架实验
│   ├── hertzserver/       # CloudWeGo Hertz
│   └── sse/               # Server-Sent Events（go-kratos）
├── ai/                # AI/LLM 实验
│   └── eino/              # CloudWeGo Eino：ADK 编排、RAG、向量检索、工具调用
└── demos/             # 其他独立 demo
    ├── http3/             # QUIC / HTTP3
    ├── oss-lab/           # 阿里云 OSS
    ├── tracing/           # OpenTelemetry 链路追踪
    ├── trtc-demo/         # 腾讯云 TRTC
```

## 说明

- 主模块只包含 `algorithms/` 和 `engineering/`，其余目录均为独立 go module，单独构建
- 验证主模块用 `go vet ./...`（部分子目录包名与目录名冲突，`go build` 会误报）
- 笔记新增遵循体例：`NN-主题.md` 分册 + README 索引 + experiments 实验
