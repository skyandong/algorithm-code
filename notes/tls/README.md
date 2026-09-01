# TLS 学习笔记

> 目标：把 TLS 从"背面试题"变成"真的懂"。先读拆解版建立直觉，再跑实验眼见为实，最后回到深度文档查漏。

## 目录

### 拆解版（通俗优先，建议按序读）

1. [为什么需要 TLS](01-为什么需要TLS.md) — 三个威胁、三种武器、混合加密的智慧
2. [TLS 1.2 握手](02-TLS1.2握手.md) — 四封信拆开讲，每封为什么存在
3. [TLS 1.3 握手](03-TLS1.3握手.md) — 快在哪、安全在哪、为什么砍功能
4. [会话恢复与 0-RTT](04-会话恢复与0-RTT.md) — 从 Session ID 到 PSK，重放攻击
5. [证书体系](05-证书体系.md) — 信任链、验证组合拳、两大生产事故
6. [密码学基础](06-密码学基础.md) — ECDHE/签名/AEAD/HKDF 各一句话吃透
7. [攻击与防御](07-攻击与防御.md) — 三类攻击面，一张速查表
8. [mTLS 与实战排查](08-mTLS与实战排查.md) — 双向认证、openssl 三板斧、MTU 大坑

### 深度版

- [TLS-Deep-Dive.md](TLS-Deep-Dive.md) — 完整深度文档（RFC 级细节 + 高级面试框架 L1~L6）

## 实验

全部自包含（本地造证书、本地起服务），无需网络：

```bash
go run ./experiments/            # 全部
go run ./experiments/ handshake  # 1. 观察 TLS1.3/1.2 协商结果
go run ./experiments/ certs      # 2. 证书链验证（1 成功 + 4 失败场景）
go run ./experiments/ resume     # 3. 会话恢复 vs 全新握手
go run ./experiments/ mtls       # 4. mTLS 双向认证
go run ./experiments/ fail       # 5. 握手失败的四种典型报错
```

| 实验 | 眼见为实什么 |
|------|-------------|
| handshake | 版本协商、密码套件名、证书链 |
| certs | `unknown authority` / `expired` / 域名不匹配长什么样 |
| resume | `DidResume=true`——第二次连接真的走了 PSK |
| mtls | 服务器视角读出客户端身份；匿名客户端被拒 |
| fail | 生产排查错误对照表的现场版 |

## 重点回顾（自测）

- [ ] 混合加密：非对称商量钥匙，对称传数据
- [ ] TLS 1.2 四封信各自干什么；Finished 为什么是安全保险
- [ ] TLS 1.3 为什么少一个 RTT（key_share 提前）
- [ ] 前向安全：ECDHE 的 E——临时密钥用完即焚
- [ ] 证书验证 = 链 + 有效期 + 域名，缺一不可
- [ ] 0-RTT 的重放风险与缓解
- [ ] mTLS = RequireAndVerifyClientCert；证书即身份
- [ ] 排查三板斧：s_client 看链 / x509 看期与名 / 指定版本试探
