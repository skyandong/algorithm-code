# TRTC 视频通话 Go 签名服务

一个基于 Go 实现的**腾讯云 TRTC 实时音视频**后端签名服务。为客户端（Web / App / 小程序）签发进房所需的鉴权凭证。

> 关键点：**RTC 音视频本身必须在客户端跑 SDK（C++/JS/原生），Go 做不了音视频采集播放。** 所以 Go 在本 demo 中的职责是标准的后端签名服务 —— 用 SDKSecretKey 计算 `UserSig` 和 `PrivateMapKey`，客户端进房前调用本服务的 API 换取凭证。这是腾讯云官方推荐的生产级做法（密钥永不下发到客户端）。

## 为什么需要这个服务

TRTC SDK 初始化/进房时要求提供凭证，凭证由 `SDKAppID + SDKSecretKey + userID` 经 HMAC-SHA256 计算得出。若把 SDKSecretKey 写死在客户端，会被反编译窃取、盗刷音视频流量。正确做法是：

```
客户端 App ──(请求 userID/roomId)──> 本 Go 签名服务 ──(返回 UserSig/PrivateMapKey)──> 客户端 ──(带凭证进房)──> TRTC
```

## 提供的接口

| 方法 | 路径 | 说明 | 参数 |
|------|------|------|------|
| GET | `/` | 前端自测页面（web/index.html） | - |
| GET | `/healthz` | 健康检查 | - |
| GET | `/api/usersig` | 只签发 UserSig 身份凭证 | `userId` 必填 |
| GET | `/api/token` | 签发完整进房凭证 UserSig + PrivateMapKey | `userId` `roomId` 必填；`expire` 可选（秒） |

### 请求示例

```bash
# 身份凭证
curl "http://localhost:8080/api/usersig?userId=alice"

# 进房凭证（数值房间号）
curl "http://localhost:8080/api/token?userId=alice&roomId=10001"
```

`/api/token` 响应：

```json
{
  "userId": "alice",
  "roomId": 10001,
  "userSig": "eJx1kG9P...",
  "privateMapKey": "eJx1kmFv...",
  "sdkAppId": 1400000000
}
```

## 快速开始

### 1. 准备云端

1. 登录[腾讯云实时音视频控制台](https://console.cloud.tencent.com/trtc)，创建应用，拿到 **SDKAppID**（形如 `1400000000`）。
2. 在「应用信息」中获取 **SDKSecretKey**（签名密钥）。
3. 新账号控制台可免费领取 **1 万分钟**新手时长包，跑 demo 足够。

### 2. 配置环境变量并启动

```bash
cd trtc-demo

export TRTC_SDKAPPID=1400000000          # 必填，你的应用 ID
export TRTC_SECRETKEY=your_secret_key    # 必填，签名密钥
export TRTC_EXPIRE=604800                # 可选，票据有效期秒，默认 7 天
export PORT=8080                          # 可选，监听端口，默认 8080

go run ./cmd/trtc-demo
```

### 3. 自测（Web 页面）

服务启动后，用浏览器打开 **http://localhost:8080/** 即可进入自测页面。

**自测方法**：开两个浏览器窗口（或两台设备），填同一份配置：
- `SDKAppID`：你的应用 ID
- `签名服务地址`：`http://localhost:8080`
- `用户ID`：**两个窗口必须不同**（如 `alice` / `bob`）
- `房间号`：**两个窗口必须相同**（如 `10001`）

两个窗口都点「进房通话」，即可看到双方视频画面互打。

> 页面里的 TRTC Web SDK 通过腾讯云官方 CDN 加载。若加载失败，请确认网络可访问 `web.sdk.qcloud.com`。

### 4. 命令行验证签名

```bash
curl "http://localhost:8080/api/token?userId=alice&roomId=10001"
```

## 对接前端

前端拿到凭证后，将其传给 TRTC SDK：

- **Web (TRTC Web SDK)**：`userSig` 传给 `createClient` 的 `genTestUserSig` 位置，`roomId` 传给进房参数。
- **App (原生/Flutter/RN)**：新版 SDK 进房需要同时传 `userSig` 和 `privateMapKey`（即本服务 `/api/token` 返回的两个字段）。

> 若前端需要**字符串房间号**，可调用 `sig.StringRoomToken`（见 `sig/sig.go`），返回的凭证中 `roomIdStr` 即字符串房间标识。

## 项目结构

```
trtc-demo/
├── cmd/trtc-demo/main.go   # 程序入口，环境变量加载 + HTTP 启动
├── config/config.go        # 配置结构体，从环境变量读取
├── server/server.go        # HTTP API 层（CORS、参数校验、路由、静态托管）
├── web/index.html          # 前端自测页面（TRTC Web SDK + 对接签名接口）
├── sig/sig.go              # 签名核心：UserSig / PrivateMapKey 生成
└── sig/sig_test.go         # 签名单元测试（用官方 VerifyUserSig 校验）
```

## 测试

```bash
go test ./...
```

测试使用官方 `tencentyun.VerifyUserSig` / `VerifyUserSigWithBuf` 校验生成的签名，确保 HMAC-SHA256 计算正确、过期逻辑符合预期。

## 安全说明

- 本服务只依赖环境变量取密钥，**不要**把 SDKSecretKey 写进代码或提交进 git。
- 生产环境建议：加业务鉴权（登录态）、限制 userId 合法来源、开启 TRTC「启动权限密钥」开关以启用 PrivateMapKey 严格校验。
- 纯 demo 可先用 `/api/usersig`；要严格控制房间权限请用 `/api/token`（带 PrivateMapKey）。
