# HTTP/1.1 与连接复用

> **核心认知：** HTTP/1.1 相对于 1.0 最重要的改变不是新方法，而是**默认持久连接**——把"每个请求一条 TCP"的代价（握手 + 慢启动）摊销掉。但 1.1 有一个改不掉的病：**响应必须按请求顺序返回**，所以并发只能靠开多条连接，而连接一多，握手的代价又回来了。

---

## 1. 报文格式：文本协议的全部

```http
POST /api/orders?debug=1 HTTP/1.1      ← 请求行：方法 目标 版本
Host: api.example.com                  ← 唯一必需的头（1.1 起）
Content-Type: application/json
Content-Length: 27                     ← 或者 Transfer-Encoding: chunked
Connection: keep-alive

{"sku": "A-1", "qty": 2}               ← 空行之后是 body
```

| 方法 | 幂等 | 安全 | 说明 |
|------|------|------|------|
| GET | ✅ | ✅ | 只读；**不应带 body**（带了也不保证被处理） |
| HEAD | ✅ | ✅ | 只要头不要体，探测存在性/长度 |
| PUT | ✅ | ❌ | 全量替换，重复执行结果相同 |
| DELETE | ✅ | ❌ | 幂等的删除 |
| POST | ❌ | ❌ | 通常用于创建/提交，**不幂等**——重试要额外去重 |

**幂等性为什么重要**：它是自动重试的前提。网关、SDK、负载均衡的重试策略都按幂等性分级——GET/PUT 可以无条件重试，POST 必须靠幂等键。

状态码只要记住族的语义边界：`1xx` 中间态、`2xx` 成功、`3xx` 需要额外动作、`4xx` 你的错、`5xx` 我的错。真正的考点在几个具体的：301 vs 302、304、429、503 + Retry-After。

---

## 2. 持久连接：1.1 的默认行为

| | HTTP/1.0 | HTTP/1.1 |
|---|---|---|
| 默认 | 每个请求一条连接，响应后关闭 | **默认复用**，靠 `Connection: close` 主动声明关闭 |
| 复用依据 | — | 响应必须能界定结尾（Content-Length 或 chunked） |

**为什么 Content-Length 是硬约束**：连接复用意味着"这条响应结束后，下一个字节就是下一条响应"。如果客户端不知道 body 有多长，就分不清边界，只能靠关闭连接来界定（1.0 的做法），复用立刻破功。所以：

- 知道长度 → `Content-Length`
- 不知道（流式生成）→ `Transfer-Encoding: chunked`，每块带长度，最后一块长度 0 结束
- 两者都没有且不是 204/304 → 只能关连接

**代理改坏 body 的经典事故**：压缩后又保留了旧的 Content-Length，客户端按长度多读/少读，下一条响应直接错位（"响应串包"）。排查时看 `Content-Length` 与实际字节数是否一致。

---

## 3. 队头阻塞：1.1 的原罪

**HTTP 层队头阻塞**：一条连接上请求必须串行（响应必须按请求顺序回来），前一个请求慢，后面全等着。

**TCP 层队头阻塞**：即使并发开多条连接，每条连接内部丢一个包，后续字节（包括已经到达的）都要等重传。

```
时序：请求 A(慢 500ms) 请求 B(快)
一条连接：  [A .......... 500ms ..........][B]
           → B 白等 500ms

多条连接：  conn1 [A .......... 500ms]
           conn2 [B]              → 不受影响（但多付了握手 + 慢启动）
```

所以业界在 1.1 时代的实际做法是 **域名分片 + 每域名 6 条连接**（浏览器约定），本质是"用连接数换并发"——代价是握手、慢启动、内存各来 6 份。

**Pipelining 为什么死了**：协议上允许一次发多个请求，但响应必须按序返回，队头阻塞一点没解决；中间件（代理、CDN）对 pipelining 支持极差，还有请求走私风险。浏览器全部放弃。

---

## 4. 缓存协商：减少请求，而不是加快请求

| 头 | 作用 |
|----|------|
| `Cache-Control: max-age=60` | 强缓存：60 秒内不发请求 |
| `Cache-Control: no-cache` | **可以存但每次要验证**（不是不缓存，这是最常被误解的一个） |
| `Cache-Control: no-store` | 真不存 |
| `ETag` / `If-None-Match` | 内容指纹，命中返回 **304 无 body** |
| `Last-Modified` / `If-Modified-Since` | 时间戳版，精度只到秒 |
| `Vary` | 同一 URL 因某头不同而不同（如 `Vary: Accept-Encoding`），缓存键要带上它 |

**`no-cache` ≠ 不缓存**，`no-store` 才是不缓存。这个坑在面试里一问一个准。

304 的价值：省掉 body 传输，但**仍然要一次往返**。所以强缓存（max-age）才是性能主力，协商缓存只是兜底。

---

## 5. 从 Go 侧看复用

Go 的 `http.Transport` 就是连接池：`MaxIdleConnsPerHost`（默认 2，非常小）、`IdleConnTimeout`（默认 90s）、`MaxConnsPerHost`（默认 0 = 不限）。

```go
tr := &http.Transport{
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 32,   // 默认只有 2，高并发下会退化成频繁建连
    IdleConnTimeout:     90 * time.Second,
    ForceAttemptHTTP2:   true,
}
client := &http.Client{Transport: tr}
```

**两个高频线上问题**：

1. **`resp.Body` 没读干就 Close** → 连接不可复用，池子里全是"脏连接"，QPS 一高就疯狂建连（表现为 TIME_WAIT 暴涨 + 延迟抖动）。正确做法：`io.Copy(io.Discard, resp.Body)` 再 Close，或至少读完全部。
2. **`MaxIdleConnsPerHost` 用了默认 2** → 并发 100 时 98 个请求要新建连接，握手 + 慢启动全吃一遍。

验证复用是否生效，用 `httptrace` 看 `GotConnInfo.Reused`：连续两次请求第二次应为 `true`。

---

## 6. 记忆锚点（面试 30 秒版）

> "HTTP/1.1 相对 1.0 的核心是默认持久连接，把握手和慢启动摊掉；能复用是因为有了 Content-Length 或 chunked 来界定响应边界。它的病是队头阻塞——一条连接上响应必须按请求顺序回，所以浏览器只好用域名分片开 6 条连接，代价是握手翻倍。pipelining 解决不了这个，因为它只解决请求发送、不解决响应顺序，所以被放弃了。"

---

## 实验

`go run ./experiments/ http1` 用 httptrace 观察连接复用：

- 默认 Transport 连续两次 GET：第二次 `Reused=true`，`Connect` 阶段不再发生
- `DisableKeepAlives=true`：每次都是新连接，`Connect` 耗时出现两次
- 不读干 body 就 Close：复用失效，与上面对比明显

对应代码：[experiments/04_http1_keepalive.go](experiments/04_http1_keepalive.go)
