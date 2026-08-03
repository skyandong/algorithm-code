# OSS Lab — 阿里云 OSS 实战练习

使用 Go 语言对接阿里云对象存储 OSS，从基础操作到高级场景全覆盖。

## 目录结构

```
oss-lab/
├── main.go                # 入口：配置加载 + 演示入口
├── config.go              # 环境变量配置
├── go.mod
├── basic/
│   └── client.go          # 基础操作：上传/下载/删除/列举/元数据/复制
├── advanced/
│   └── client.go          # 高级操作：分片上传/断点续传/并发上传
├── presign/
│   └── client.go          # 签名 URL：临时授权上传/下载
└── examples/
    └── scenarios.go       # 实战场景：文件服务/静态网站/流式导出/生命周期
```

## 快速开始

### 1. 设置环境变量

```bash
export OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com
export OSS_ACCESS_KEY_ID=LTAI5txxxxxxxxxxxxx
export OSS_ACCESS_KEY_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxx
export OSS_BUCKET_NAME=my-test-bucket
```

### 2. 安装依赖

```bash
cd oss-lab
go mod tidy
```

### 3. 运行

```bash
go run .
```

## 功能清单

### 基础操作 (`basic/`)

| 功能 | 方法 | 说明 |
|------|------|------|
| Bucket 列表 | `ListBuckets()` | 列举所有 Bucket |
| 创建 Bucket | `CreateBucket()` | ACL 可设为私有/公共读 |
| 字符串上传 | `PutFromString()` | 直接上传字符串内容 |
| 文件上传 | `PutFromFile()` | 上传本地文件 |
| Reader 上传 | `PutFromReader()` | 最通用的上传方式 |
| 下载到字符串 | `GetToString()` | 小文件下载 |
| 下载到文件 | `GetToFile()` | 下载到本地文件 |
| 流式读取 | `GetReader()` | 适合大文件，不占内存 |
| 删除 | `Delete()` | 删除单个对象 |
| 存在性检查 | `Exists()` | 判断对象是否存在 |
| 列举 | `ListObjects()` | 支持前缀、分页 |
| 复制 | `Copy()` | Bucket 内复制 |
| 元数据 | `SetMeta()` / `GetMeta()` | 设置/获取自定义元数据 |

### 高级操作 (`advanced/`)

| 功能 | 方法 | 面试重点 |
|------|------|---------|
| 分片上传 | `MultipartUpload()` | Initiate → UploadPart → Complete 三阶段 |
| 断点续传 | `ResumableUpload()` | checkpoint 机制，中断恢复 |
| 并发分片上传 | `ConcurrentMultipartUpload()` | goroutine + semaphore 控制并发 |
| 断点续传下载 | `ResumableDownload()` | 大文件下载 |

### 签名 URL (`presign/`)

| 功能 | 说明 |
|------|------|
| `GetPresignedURL()` | 临时下载链接 |
| `PutPresignedURL()` | 客户端直传（不经过后端） |
| 带 Content-Type | 控制浏览器下载行为 |

### 实战场景 (`examples/`)

| 场景 | 核心技巧 |
|------|---------|
| **文件存储服务** | 按日期分目录、生成临时下载链接 |
| **静态网站部署** | walk 目录上传、自动 Content-Type |
| **大数据流式导出** | `io.Pipe` 连接 CSV 生成和 OSS 上传，不落盘 |
| **生命周期管理** | 自动过期删除、自动转 IA/Archive |

## API 速查表

### OSS 操作选择

```
文件大小 < 100MB  → PutObject（简单上传）
文件大小 > 100MB  → MultipartUpload（分片上传）
网络不稳定       → ResumableUpload（断点续传）
需要加速         → ConcurrentMultipartUpload（并发分片）
临时分享         → GetPresignedURL（签名 URL）
客户端直传       → PutPresignedURL（签名 PUT URL）
流式数据         → io.Pipe + PutObject（流式上传）
```

### Bucket ACL

| ACL | 常量 | 说明 |
|-----|------|------|
| 私有 | `oss.ACLPrivate` | 仅 Owner 可读写 |
| 公共读 | `oss.ACLPublicRead` | 所有人可读，Owner 可写 |
| 公共读写 | `oss.ACLPublicReadWrite` | ⚠️ 不推荐 |

## 面试自测

写完代码后，试着回答：

1. **分片上传的三阶段是什么？** 如果第二阶段某片失败了怎么处理？
2. **断点续传的 checkpoint 里存了什么？** 为什么能"续传"？
3. **签名 URL 的安全原理？** 为什么不能让客户端直接知道 AccessKey？
4. **`io.Pipe` 在流式导出中的作用？** 如果不落盘，内存会爆吗？
5. **生命周期规则** 和 **手动删除** 的区别？什么时候用哪种？
6. 如果要实现**跨区域复制（CRR）**，怎么设计？
