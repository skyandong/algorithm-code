# nginx 学习笔记

> 目标:系统掌握 nginx 反向代理/负载均衡/限流/TLS 终止,以"边缘代理"视角串联已有笔记(tls/redis/oss-lab)。
> **模块铁律:TLS 终止只在 nginx 边缘做,后端 web 服务只说纯 HTTP,代码里没有任何加密解密 —— 后端感知 https 全靠 X-Forwarded-Proto 透传头。**

## 目录

1. [核心架构与配置](01-核心架构与配置.md) — master-worker、epoll、location 优先级、热重载
2. [反向代理与负载均衡](02-反向代理与负载均衡.md) — proxy_pass 四种写法、upstream 策略、透传头、keepalive、502/504
3. [限流与安全](03-限流与安全.md) — 漏桶 rate/burst/nodelay、多层限流、安全加固清单
4. [HTTPS 与 TLS 终止实战](04-HTTPS-TLS终止实战.md) — TLS 终止架构、自签证书、后端零 TLS 的可验证形态
5. [性能调优与生产排查](05-性能调优与排查.md) — sendfile/gzip、buffering、502/慢请求排查手册
6. [场景题专题](06-场景题专题.md) — 统一入口、灰度发布、秒杀入口、多级代理真实 IP
7. [最小 API 网关](07-最小API网关.md) — 用 auth_request/limit_req/map 把网关拼出来、身份透传、网关联不是安全边界
8. [面试一口答](面试一口答.md) — 考前速刷:30 个高频问题"张口就来"

## 重点回顾(自测)

- [ ] master-worker 分工 + 为什么 worker 数 = 核数;reload 不丢请求的完整过程
- [ ] location 五种语法优先级 + `^~` 的存在意义
- [ ] proxy_pass 带斜杠/不带斜杠的区别(高频坑)
- [ ] upstream 四种策略 + max_fails/fail_timeout 被动健康检查
- [ ] proxy_set_header 四件套;X-Forwarded-For 为什么最左不可信、怎么正确取真实 IP
- [ ] upstream keepalive 三件套(keepalive 32 + proxy_http_version 1.1 + Connection "")
- [ ] limit_req 漏桶:rate/burst/nodelay 三个参数的精确语义;burst 加不加 nodelay 的区别
- [ ] limit_req_zone 用 $binary_remote_addr 的原因(10MB ≈ 16 万 IP)
- [ ] **TLS 终止架构:证书只在 nginx;后端对 8443 是明文;X-Forwarded-Proto 是协议感知唯一途径**
- [ ] 自签证书三易错点:SAN 必须有、-nodes、curl -k
- [ ] sendfile 零拷贝链路;SSE 必须 proxy_buffering off
- [ ] 502/504 区别 + access log 双时间差定位法
- [ ] POST 重试幂等炸弹(proxy_next_upstream)
- [ ] **auth_request 子请求机制:为什么鉴权端点必须加 `internal`**
- [ ] **`auth_request_set $var $upstream_http_x_xxx` —— 抓鉴权响应头注入后端,让后端免验签**
- [ ] **先鉴权再限流的顺序理由:假 token 不该消耗限流配额**
- [ ] **`proxy_pass http://$var` 带变量时 upstream keepalive 失效(动态路由的代价)**
- [ ] **`split_clients` 按百分比灰度 vs `map` 按 header 灰度,各自适用场景**
- [ ] **网关是流量入口不是安全边界:绕过直连后端时鉴权完全不生效**

## 跑实验

**方式一:本机 nginx(brew install nginx)**

```bash
cd notes/nginx/conf
./gen-certs.sh                       # 生成自签证书(仅首次)
mkdir -p logs run
nginx -p "$(pwd)/" -c nginx.conf     # 起在 8080(反代)/8083(限流)/8443(TLS);-p 必须,否则相对路径按安装目录解析
cd ../experiments
go run .                             # 起两个纯 HTTP 后端(8081/8082)并跑全部实验
nginx -p "$(pwd)/../conf/" -c nginx.conf -s stop   # 结束
```

**方式二:Docker(镜像拉不下来先手动 `docker pull nginx:1.27-alpine`)**

```bash
cd notes/nginx/conf
./gen-certs.sh && mkdir -p logs
docker run --rm --name nginx-lab \
  -p 8080:8080 -p 8083:8083 -p 8443:8443 \
  -v "$(pwd)/nginx-docker.conf":/etc/nginx/nginx.conf:ro \
  -v "$(pwd)/certs":/etc/nginx/certs:ro \
  -v "$(pwd)/logs":/var/log/nginx \
  nginx:1.27-alpine
# 另一个终端跑 experiments,同上
docker stop nginx-lab
```

**文件说明**

| 文件 | 内容 |
|------|------|
| `conf/nginx.conf` | 主实验配置(本机版,upstream=127.0.0.1),一个文件覆盖 01-05 篇 |
| `conf/nginx-docker.conf` | Docker 版,upstream 用 host.docker.internal(容器内 127.0.0.1 是容器自己) |
| `conf/gateway.conf` | **第七节最小网关**(本机版):auth_request 鉴权 + limit_req + map 灰度 + 身份透传 |
| `conf/gateway-docker.conf` | 第七节 Docker 版,已在 nginx:1.27-alpine 上实测跑通 |
| `conf/gen-certs.sh` | 自签证书生成(原理在 notes/tls/05) |
| `experiments/` | Go 验证脚本 + 两个零 TLS 后端,`go run .` 全跑,`go run . 7` 只跑网关一节 |

**第七节单独跑(网关需手动起 nginx)**

```bash
cd notes/nginx/experiments
go run . 7            # 起 8071(v1)/8072(v2) 后端 + 9090 鉴权服务
                      # 缺 nginx 时会打印启动指引而不是崩掉

# 另开终端,Docker 起 nginx(不用装)
cd ../conf && mkdir -p logs
docker run --rm --name gw-lab -p 8090:8090 \
  -v "$(pwd)/gateway-docker.conf":/etc/nginx/nginx.conf:ro \
  -v "$(pwd)/logs":/var/log/nginx \
  nginx:1.27-alpine
```

## 与其他模块的衔接

- `notes/tls` — 证书体系/握手原理(本篇只管"怎么用")
- `notes/redis 08/11` — 会话共享与全局限流(Redis + Lua)
- `engineering/consistenthash` — 一致性哈希在负载均衡里的应用
- `web/sse` — 反代 SSE 必须 `proxy_buffering off` 的实战
- `notes/kafka` — 接入层限流思想同源:把脏流量挡在最外面
