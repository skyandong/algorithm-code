# mTLS 与实战排查

> **核心认知：mTLS = 把"验明正身"从单边变双边——服务器也要验客户端的证书。
> 它是零信任网络和服务网格的身份底座：证书即身份。
> 排查则是另一门功夫：openssl s_client 一把梭，先看链、再看期、再对名。**

---

## mTLS：双向认证

普通 HTTPS 只防"假服务器"。但很多场景要防"假客户端"：

```
普通 TLS:   客户端 ──验服务器证书──▶ 服务器        防假服务器
mTLS:       客户端 ──验服务器证书──▶ 服务器
            客户端 ──呈上客户端证书──▶ 服务器        同时防假客户端
```

实现只差一行配置（Go）：

```go
// 服务器端
tls.Config{
    ClientAuth: tls.RequireAndVerifyClientCert, // 强制要求+验证客户端证书
    ClientCAs:  clientRootPool,                  // 验证客户端证书用的 CA
}
```

握手流程的变化：服务器在握手中间加发一条 `CertificateRequest`，客户端回自己的证书。服务器验证通过才继续。

### 谁在用 mTLS

- **Istio/服务网格**：每个服务的 sidecar 互验证书，服务间调用天然可信
- **零信任网络**：没有"内网即可信"，每次连接都要双向亮证
- **企业 API**：合作方调接口必须持证书，比 API key 更硬

### 大规模 mTLS 的核心矛盾：证书生命周期

1000 个服务的集群，人肉管证书是灾难。解法是工程化的：

```
Root CA（离线，10 年）           信任的根基，永不上网
  └─ Intermediate CA（在线）      日常签发
       ├─ 服务端证书: 24h 过期，到期自动轮转
       └─ 客户端证书: 72h 过期
```

**短有效期是灵魂**：证书活不过 24 小时 → "吊销"这个老大难问题被"等它过期"取代 → 不再需要笨重的 CRL/OCSP 基础设施。K8s 生态里 cert-manager / Vault / SPIFFE 干的就是自动签发+轮转这件事。

**身份编码**：证书 SAN 里写 `spiffe://cluster/ns/payments/sa/api` 这样的 URI——服务身份与 IP/端口解耦，授权引擎（如 OPA）按 SPIFFE ID 判断"谁能调谁"。

---

## 实战排查：三板斧

### 斧1：openssl s_client —— 看服务器到底发了什么

```bash
openssl s_client -connect example.com:443 -servername example.com -showcerts
```

看三处：
- **证书链张数**：`depth=0,1,2` 才正常；只有 depth=0 = 服务器漏发中间证书（高频事故）
- **verify return code**：`0 (ok)` 之外都有问题
- **协议与套件**：确认服务器实际支持的版本

### 斧2：openssl x509 —— 看证书细节

```bash
echo | openssl s_client -connect host:443 2>/dev/null | openssl x509 -noout -dates -ext subjectAltName
```

- 过没过期（`notAfter`）
- 域名在不在 SAN 里

### 斧3：指定版本/套件试探

```bash
openssl s_client -connect host:443 -tls1_2     # 只用 1.2 试，握手失败=服务器不支持
openssl s_client -connect host:443 -tls1_3     # 只用 1.3 试
```

---

## 错误对照表（背这张就够）

| 报错关键词 | 原因 | 动作 |
|-----------|------|------|
| `certificate signed by unknown authority` | 链断/CA 不被信任/漏发中间证书 | 查 fullchain 配置、装 CA 证书 |
| `certificate has expired` | 过期 | 续期 + 检查自动续期任务是否挂了 |
| `certificate is valid for X, not Y` | 域名不在 SAN | 重签含正确域名的证书 |
| `protocol version not supported` | 版本不重叠 | 升级客户端或放开服务端版本 |
| `handshake failure` | 无共同密码套件 | 调整两端套件配置 |

## 一个隐蔽大坑：MTU 导致握手超时

**症状**：部分用户（尤其移动端）访问 HTTPS 间歇性超时，同一批用户访问 HTTP 正常。

**原理**：

```
会超 MTU 的是服务器→客户端方向的证书链（Certificate 消息，RSA 4096 能到 4KB+）
  → 某些链路 MTU < 1500（如移动网 1280）
  → IP 分片 → 很多 NAT/网关直接丢弃分片包
  → 客户端等不到服务器响应 → 超时
ClientHello 通常仅 200-500 字节，一般不是元凶（只有链路 MTU 极小时才可能出问题）
```

**排查**：`ping -M do -s 1472 host` 不通就逐步往下试，找到路径 MTU。

**修复**：换 ECC 证书（ECDSA P-256 叶子证书约 0.6-0.8KB，比 RSA 4096 的约 2KB 小 2-3 倍，而非一个数量级）、精简密码套件列表、服务器配 MSS clamping。

---

## 对应实验

跑 `go run ./experiments/ mtls`：
- 场景1：客户端持证书连接成功，**服务器侧打印出客户端身份 CN=service-a**——"证书即身份"眼见为实
- 场景2：不带证书的客户端被服务器用 alert 拒绝（`certificate required`）
- 顺带观察一个 1.3 细节：拒绝发生在客户端第一次写数据时，而不是 Dial 时（握手异步性）

跑 `go run ./experiments/ fail`：错误对照表的左侧四种全部亲手触发一遍。

对应代码：[experiments/04_mtls.go](experiments/04_mtls.go)、[experiments/05_failures.go](experiments/05_failures.go)

深入版：[TLS-Deep-Dive.md](TLS-Deep-Dive.md) 第 8、9 章
