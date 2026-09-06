# 七、用 nginx 拼一个最小 API 网关

> 这一篇不是讲"网关理论"，是讲**用几条 nginx 指令把网关拼出来**，每条指令为什么这么写、踩过什么坑。
> 配置在 `conf/gateway.conf`（本机）/ `conf/gateway-docker.conf`（Docker），实验在 `experiments/07_gateway.go`。
> 全部结论都在本机 + nginx:1.27-alpine 容器上跑过，数据可直接复现。

## 1. 网关在 nginx 上就是四条指令

| 网关能力 | nginx 指令 | 一句话原理 |
|---|---|---|
| 统一鉴权 | `auth_request` | 每笔请求先发一个子请求问鉴权服务，只看返回码 |
| 限流 | `limit_req` | 漏桶，超配额直接 429，后端一次都收不到 |
| 灰度路由 | `map` + 变量 `proxy_pass` | 按请求头/百分比决定上游 |
| 身份透传 | `auth_request_set` + `proxy_set_header` | 鉴权结果抓成变量，注入给后端 |

没有魔法。所谓"网关"，本质就是**把每个服务都要写一遍的横切逻辑，用反向代理的指令表达出来**。

## 2. auth_request：子请求机制

```
客户端 ──GET /api/hello──▶ nginx ──子请求──▶ 鉴权服务 /auth
                            │                    │
                            │◀──── 200 / 401 ────┘
                            │
             200 → 继续反代到后端
             401 → 直接返回，后端根本不知道有请求来过
```

三个必须写对的点：

**① `internal` 不能少**

```nginx
location = /_gw_auth {
    internal;                 # 关键
    proxy_pass http://auth_svc/auth;
}
```

少了这行，外部可以直接 `curl /_gw_auth` 问鉴权服务"我合不合法"——等于把鉴权端点裸奔出去。
实测：加了 `internal` 后直连返回 **404**。

**② 子请求不带 body**

```nginx
proxy_pass_request_body off;
proxy_set_header Content-Length "";
```

鉴权只看 header，带 body 过去纯属浪费带宽，还可能把大文件上传的 body 读两遍。

**③ `auth_request_set` 才是最值钱的一行**

```nginx
auth_request_set $gw_user $upstream_http_x_user_id;
auth_request_set $gw_tier $upstream_http_x_user_tier;
proxy_set_header X-Gateway-User $gw_user;
...
```

它把鉴权服务**响应头**里的东西抓成 nginx 变量，再注入给后端。
于是后端**不用验签**——网关验一次，后端直接用：

```json
{"gw_user":"1001","gw_tier":"gold","version":"v1"}
```

这就是"网关做鉴权、服务做业务"的落地形态。少这一行，后端还是得自己解析 token，网关就白搭了。

## 3. 顺序：先鉴权再限流

```nginx
auth_request /_gw_auth;
error_page 401 = @gw_unauthorized;

limit_req zone=gw_limit burst=5 nodelay;
error_page 429 = @gw_toomany;
```

顺序有讲究：**未登录的请求连限流配额都不该消耗**。否则攻击者用一堆假 token 就能把限流桶打满，正常用户反而进不来。

统一错误形状也值得做——别让后端各说各话：

```nginx
location @gw_unauthorized {
    default_type application/json;
    return 401 '{"code":401,"msg":"gateway: token missing or invalid"}';
}
```

## 4. 灰度：两种写法，同一个坑

**按 header（精确可控，适合内部测试 / 给特定用户开）**

```nginx
map $http_x_canary $gw_backend {
    default  backend_v1;
    "1"      backend_v2;
}
proxy_pass http://$gw_backend;
```

**按百分比（放量用，同一用户稳定落在同一版本）**

```nginx
split_clients "${remote_addr}${request_uri}" $gw_pct {
    10%  backend_v2;
    *    backend_v1;
}
```

**⚠️ 共同的坑**：`proxy_pass` 里一旦带变量，nginx 就没法复用上游长连接，`upstream` 里的 `keepalive` 直接失效，每次请求重新建连。
这是"动态路由"换来的代价。要么接受，要么上 OpenResty 的 `balancer_by_lua`。

实测灰度生效：默认走 `backend_v1`，带 `X-Canary: 1` 走 `backend_v2`。

## 5. 网关不是安全边界（最重要的认知）

实测：绕过网关直连后端 8071，返回 200，且 `gw_user` 是空的——**鉴权压根没生效**。

```
curl http://127.0.0.1:8071/api/hello
→ {"gw_user":"","gw_tier":"","version":"v1"}   200 OK
```

网关只是"流量入口"，不是"安全边界"。真正的隔离必须靠：

- 网络层：后端只监听内网网卡，公网路由不可达
- 服务网格：mTLS 双向认证，服务间互相验身份
- 主机层：安全组 / 防火墙

**别指望应用层网关兜底**。面试被问"后端怎么防止被绕过网关直连"，答"网关拦着"是错的。

## 6. 面试怎么答

**Q：有了 nginx 为什么还要 API 网关？**
nginx 改路由要动 conf + reload；网关能对接注册中心动态发现、配置热更新、插件即插即用。两者生产上是叠着用的：nginx 管边缘 TLS 和四层，网关管七层业务路由和治理。

**Q：鉴权放网关还是服务里？**
都做，但分工不同。网关验 token 的**有效性**（签名、过期），验完把 user_id 注入 header；服务做**细粒度权限**（这个用户能不能操作这个资源）。网关不适合做后者——那是业务逻辑，塞进网关会让网关变成最大的单体。

**Q：限流放网关还是服务端？**
都要。网关做粗粒度防刷（按 IP / appkey），把脏流量挡在最外面；服务端做精细限流（按用户、按接口、按配额）。只有网关限流，内部调用和绕过就裸奔了。

**Q：网关挂了怎么办？**
网关本身必须集群 + 健康检查，不能有单点。另外要设计降级：鉴权服务超时时是放行还是拒绝——`auth_request` 的子请求失败默认返回 500，生产上要显式决定这个分支。

## 本篇对应实验

```bash
cd notes/nginx/experiments
go run . 7            # 起 v1/v2 后端 + 鉴权服务，跑第七节（缺 nginx 时会打印启动指引）

# 另开终端起 nginx（Docker 方式，不用装）
cd ../conf && mkdir -p logs
docker run --rm --name gw-lab -p 8090:8090 \
  -v "$(pwd)/gateway-docker.conf":/etc/nginx/nginx.conf:ro \
  -v "$(pwd)/logs":/var/log/nginx \
  nginx:1.27-alpine
```

| 端口 | 角色 |
|---|---|
| 8090 | 网关入口 |
| 8071 / 8072 | v1 / v2 后端（灰度目标） |
| 9090 | 鉴权服务 |

实测结果：无/错 token → 401；正确 token → 200 且后端拿到 `gw_user=1001`；
`/_gw_auth` 直连 → 404；`X-Canary:1` → v2；连打 15 次 → 200×6 / 429×9。
