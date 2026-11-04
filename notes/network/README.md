# 网络学习笔记

> 目标：把 TCP/HTTP 从"背八股"变成"能定位"。7 篇按「传输层 → 应用层 → 排查」推进，每篇配可运行实验（本地起服务，无需外网）。

## 目录

1. [TCP 报文与三次握手](01-TCP报文与三次握手.md) — 报文头逐字段、两个队列、SYN flood
2. [四次挥手与连接状态](02-四次挥手与连接状态.md) — 状态机、TIME_WAIT/CLOSE_WAIT、RST 归因
3. [可靠传输与拥塞控制](03-可靠传输与拥塞控制.md) — RTO、快速重传、流控 vs 拥塞控制、CUBIC/BBR
4. [HTTP/1.1 与连接复用](04-HTTP1.1与连接复用.md) — 持久连接、队头阻塞、缓存协商
5. [HTTP/2 多路复用](05-HTTP2多路复用.md) — 二进制分帧、HPACK、两级流控、残留的 TCP 队头阻塞
6. [HTTP/3 与 QUIC](06-HTTP3与QUIC.md) — 用户态传输、0-RTT、连接迁移、为什么用 UDP
7. [排查实战](07-排查实战.md) — 五段定位法、症状速查表、抓包时机

## 实验

全部本地自包含（本地监听 + 本地回环），无需外网、无需 root：

```bash
go run ./experiments/              # 全部
go run ./experiments/ handshake    # 1. 建连成功与两种失败（refused vs timeout）
go run ./experiments/ close        # 2. 半关闭、TIME_WAIT 与 CLOSE_WAIT
go run ./experiments/ congestion   # 3. 接收窗口耗尽导致写阻塞
go run ./experiments/ http1        # 4. keep-alive 复用与失效场景
go run ./experiments/ http2        # 5. ALPN 协商 h2 与单连接多路复用
go run ./experiments/ quic         # 6. 四元组 vs Connection ID（连接迁移）
go run ./experiments/ diagnose     # 7. 五段耗时拆解
```

| 实验 | 眼见为实什么 |
|------|-------------|
| handshake | `ECONNREFUSED`（立刻）与 `i/o timeout`（等到超时）的区别 |
| close | `CloseWrite` 后仍可读；主动关闭方留 TIME_WAIT，未 close 的一侧停 CLOSE_WAIT |
| congestion | 对端不读 → 写调用阻塞，与丢包无关 |
| http1 | `httptrace` 的 `Reused`：读干 body 才复用，否则反复建连 |
| http2 | `resp.Proto == HTTP/2.0`，并发请求共用一条连接 |
| quic | 换端口后 Connection ID 仍能找回会话，四元组查找则失效 |
| diagnose | 五段耗时占比，一眼看出慢在哪一段 |

## 重点回顾（自测）

- [ ] 三次握手各确认了什么；两个队列满了分别是什么现象
- [ ] 为什么四次挥手；TIME_WAIT 的两个作用；CLOSE_WAIT 只增不减说明什么
- [ ] RTO 计算与 Karn 算法；快速重传的触发条件
- [ ] 发送窗口 = min(rwnd, cwnd)，两者的责任方不同
- [ ] 慢启动/拥塞避免/快恢复的触发条件；超时与快重传惩罚不同
- [ ] Nagle + 延迟 ACK 的 40ms 组合；Go 默认 TCP_NODELAY
- [ ] HTTP/1.1 复用的前提（Content-Length 或 chunked）；no-cache 不等于不缓存
- [ ] HTTP/2 二进制分帧、HPACK 为何不用 gzip、TCP 层队头阻塞为何仍在
- [ ] HTTP/3/QUIC 三个卖点；为什么必须基于 UDP，代价是什么
- [ ] 五段定位法与各段对应的第一动作

## 与其它模块的关系

- 传输层安全（TLS 握手、证书）见 [tls](../tls/README.md)
- 反向代理与 TLS 终止的落地配置见 [nginx](../nginx/README.md)
- 可观测性与延迟分解的服务端视角见 [microservice](../microservice/README.md)
