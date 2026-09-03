# HTTPS 与 TLS 终止实战

> **本篇设计基线（模块铁律）：TLS 终止只在 nginx 边缘做。证书、私钥、协议版本、密码套件全部属于 nginx 层；后端 web 服务只监听纯 HTTP，代码里没有任何加密解密。
> 后端想知道"用户原始是不是 HTTPS"，只看 nginx 透传的 X-Forwarded-Proto 头。
> 证书怎么来、握手怎么回事 → `notes/tls`（原理层）；本篇讲"证书怎么用、终止在哪"（实战层）。**

---

## 架构：一图看懂 TLS 终止

```
client ══TLS 加密══▶ nginx :8443 ──解密──▶ 纯 HTTP :8081/:8082 ──▶ hertzserver
        证书只在这        解密后             后端全程明文
                       proxy_pass          代码零 TLS
```

**WHY 后端不做加密：**
- 加解密是 CPU 密集操作，集中在边缘一层做，后端把 CPU 留给业务
- 证书续期、协议升级（TLS1.2→1.3）、密码套件调整都是运维动作，改 nginx 配置即可，**后端代码零改动、零重新部署**
- 后端是内网服务，攻击面在边缘已被过滤，明文走内网（生产做到零信任再上 mTLS，学习阶段不需要）

**注意 Go 实验代码里的 crypto/tls**：出现在 `experiments/`（模拟客户端做 HTTPS 请求）是合理的——客户端当然要加密；原则约束的是**后端服务**不碰加密。

---

## nginx 侧配置（全部的 TLS 相关配置都在这）

```nginx
# HTTPS server：唯一的 TLS 终止点
server {
    listen 8443 ssl;                     # 本地实验用 8443，生产 443
    server_name localhost;

    ssl_certificate     conf/certs/server.crt;   # 证书链（leaf + 中间）
    ssl_certificate_key conf/certs/server.key;   # 私钥，权限 600，绝不进 git

    # 协议版本：只开 1.2/1.3（1.0/1.1 有已知漏洞）
    ssl_protocols       TLSv1.2 TLSv1.3;
    # 密码套件：1.3 的套件不受此指令控制（内置强制安全）
    ssl_ciphers         HIGH:!aNULL:!MD5;

    # 会话恢复：省一次完整握手（原理见 notes/tls/04）
    ssl_session_cache   shared:SSL:10m;
    ssl_session_timeout 10m;

    # 生产再开：OCSP 装订（nginx 帮客户端查证书吊销状态）
    # ssl_stapling on;

    location / {
        proxy_pass http://backend;
        proxy_set_header X-Forwarded-Proto $scheme;   # ★ 后端感知 https 的唯一途径
        proxy_set_header X-Real-IP         $remote_addr;
    }
}
```

**HTTP 跳转 HTTPS**（生产标配）：

```nginx
server {
    listen 8080;
    return 301 https://$host$request_uri;    # 301 永久跳转，浏览器记住
}
```

---

## 自签证书生成（衔接 notes/tls/05）

```bash
cd conf && ./gen-certs.sh    # 生成 certs/server.crt + server.key（SAN 含 localhost）
```

脚本核心一行（完整版看 `conf/gen-certs.sh`）：

```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout certs/server.key -out certs/server.crt \
    -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
```

三个易错点：
1. **必须有 SAN**（subjectAltName），现代浏览器只认 SAN 不认 CN
2. `-nodes` 不能少：nginx 读私钥不支持交互输密码（除非配 ssl_password_file）
3. curl 要加 `-k` 跳过校验（自签证书不在系统信任链），或把 server.crt 加进信任链

仓库 `.gitignore` 已全局忽略 `*.pem` `*.key`，且整个 `conf/certs/` 目录被单独忽略——私钥绝不能提交，自签 crt 每台机器重新生成也无意义。

---

## 后端侧：验证"零 TLS"（本篇实验的核心）

hertzserver / experiments 里的后端：

```go
// 后端：只有纯 HTTP，注意没有任何 TLS import
srv := &http.Server{Addr: ":8081", Handler: echoHandler()}
srv.ListenAndServe()          // 没有 ListenAndServeTLS
```

`experiments/05_https_termination.go` 做三件事：

1. 对 `:8443` 发起 TLS 握手 → 成功，且能看到证书 CN/有效期/TLS 版本
2. 对 `:8081` 发起 TLS 握手 → **失败**（后端根本不会说 TLS），证明加密确实终止在 nginx
3. 分别用 HTTP（8080）和 HTTPS（8443）访问同一接口 → 后端回显的 `X-Forwarded-Proto` 分别是 `http` 和 `https`，**后端代码一行没改**

这就是设计基线的可验证形态：**加密的开关在 nginx 配置里，不在任何一行后端代码里。**

---

## 排障速查

| 症状 | 原因 |
|------|------|
| `SSL_do_handshake() failed` | 后端不是 TLS 服务却配了 HTTPS upstream（方向搞反） |
| 浏览器报证书域名不匹配 | 没配 SAN 或访问的域名不在 SAN 列表 |
| `ssl_stapling ignored` | 自签证书无 OCSP 地址，正常现象 |
| 后端收到的还是 http 头 | 忘了 `proxy_set_header X-Forwarded-Proto $scheme;` |

---

## 本篇对应实验

`experiments/05_https_termination.go` —— TLS 握手成功 + 后端握手失败（证明终止点）+ X-Forwarded-Proto 验证
