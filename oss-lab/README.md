# OSS Lab — 统一 S3 接口的对象存储练习

用一套 **aws-sdk-go-v2 的 S3 代码**，在**本地 MinIO** 与**阿里云 OSS（S3 兼容模式）** 之间无缝切换，业务层无感知。

核心价值：验证「测试环境用本地存储，生产切阿里云 OSS 能否平滑过渡」——答案是**能，前提是统一到 S3 接口、避免厂商专有 SDK 的独有能力**。

## 目录结构

```
oss-lab/
├── main.go                  # 入口：加载配置 + 自动建桶 + 四类演示
├── go.mod
├── docker-compose.yml       # 本地 MinIO（开箱即用）
├── .gitignore               # 忽略 ./data（本地存储数据）
├── storage/
│   ├── config.go            # 多后端配置：local/minio/aliyun/aws/custom
│   └── client.go            # 抽象层：Put/Get/List/Copy/Head/Presign/Multipart 等领域方法，封装并隐藏厂商 SDK
├── basic/                   # 基础操作：上传/下载/列举/复制/元数据
├── advanced/                # 分片上传/断点续传/并发上传/断点下载
├── presign/                 # 签名 URL：临时授权上传/下载
└── examples/                # 实战场景：CSV 导出/临时下载链接/生命周期
```

## 快速开始（本地，零依赖阿里云账号）

```bash
cd oss-lab
docker compose up -d          # 启动本地 MinIO（S3 端口 9000）
go run .                      # 自动建桶并跑完基础/签名/分片/实战四类演示
```

## 切换到阿里云 OSS（仅改环境变量）

```bash
export STORAGE_BACKEND=aliyun
export STORAGE_REGION=cn-hangzhou
export STORAGE_ACCESS_KEY=<RAM 子账号 AccessKey ID>
export STORAGE_SECRET_KEY=<RAM 子账号 AccessKey Secret>
export STORAGE_BUCKET=<你的 bucket>
go run .
```

**本地与云端唯一差异就是这几行环境变量，业务代码一行不用动。**

## 抽象层设计（关键）

`storage.Client` 是唯一的「抽象边界」，也是本仓库「真抽象」的核心：

- **所有原子操作**（Put / Get / Delete / List / Copy / Head / 分片 / 签名 URL）都收敛在 `storage` 包内，用与厂商无关的「领域参数」表达意图；
- 底层 `aws-sdk-go-v2` 的 `*s3.Client`、`*s3.PresignClient` 是**私有字段**，外部包拿不到，因此 `basic / presign / advanced / examples` **完全不 import 任何厂商 SDK**，也**不手写 `s3.*Input`**；
- 切换后端（本地 MinIO / 阿里云 OSS / AWS S3）只需改 `storage/config.go` 的预设，四个业务包**零改动**。

这与「S3 协议兼容」的本质区别：兼容模式下游仍直接构造 `s3.PutObjectInput`，换非 S3 后端时这些调用要重写；真抽象下游碰不到 `s3` 类型，换后端只改 `storage` 一处。

> 验证：全仓 `grep "service/s3"` 仅出现在 `storage/` 内部，业务包零引用。

## 配置项

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `STORAGE_BACKEND` | `local`/`minio`/`aliyun`/`aws`/`custom` | `local` |
| `STORAGE_ENDPOINT` | 覆盖预设端点（内网域名、CNAME 自定义域名用这个） | 按后端预设 |
| `STORAGE_REGION` | region | aliyun 默认 `cn-hangzhou` |
| `STORAGE_ACCESS_KEY` | 访问密钥 ID | local/minio 默认 `minioadmin` |
| `STORAGE_SECRET_KEY` | 访问密钥 Secret | local/minio 默认 `minioadmin` |
| `STORAGE_BUCKET` | bucket 名称 | `oss-lab` |
| `STORAGE_PATH_STYLE` | `true`=path-style，`false`=virtual-hosted | 按后端预设 |

## ⚠️ 切换阿里云 OSS 必须知道的兼容性细节

这是「能不能平滑切」的真正决定因素，已经内置处理：

1. **两种端点别混用**
   - 专有 SDK 端点：`oss-cn-hangzhou.aliyuncs.com`（本仓库**不**用）
   - S3 SDK 端点：`s3.oss-cn-hangzhou.aliyuncs.com` ← 本仓库用这个
   - 混用会报 `SignatureDoesNotMatch`。

2. **寻址风格**：阿里云 OSS **只支持 virtual-hosted style**（`UsePathStyle=false`）；
   本地 MinIO 用 path-style。配置层已按后端预设好，无需手动干预。

3. **chunked encoding（Go 特有坑）**：OSS 不支持 `aws-chunked` 分块传输。
   因此本仓库**手写分片上传，刻意绕开 `manager.Uploader`**（它会在传大文件时自动开启 chunked）。
   普通 `PutObject` 不受影响。

4. **CRC32 校验头**：aws-sdk-go-v2 v1.40+ 默认给所有 PutObject 加 `x-amz-checksum-crc32`，
   部分第三方存储不支持。本仓库在 `storage/client.go` 显式关闭
   `RequestChecksumCalculation=WhenRequired`，由业务自己按需设置校验。

5. **ETag 大小写**：OSS 的 PUT 上传 ETag 大写、S3 小写；**分片上传 ETag 算法两者完全不同**。
   跨服务比对完整性时不要依赖 ETag。

6. **2025-03-20 起新开通的 OSS**：中国内地 Bucket 不能用默认外网域名调用上传/下载等数据类 API，
   需配置 CNAME 自定义域名后，再用 `STORAGE_ENDPOINT=https://<自定义域名>` 覆盖。

## 功能清单

### 基础操作 (`basic/`)

| 方法 | 说明 |
|------|------|
| `PutWithOptions` | 上传并指定 Content-Type 与自定义元数据 |
| `PutFromString` / `PutFromFile` | 字符串 / 文件上传 |
| `GetToString` / `GetReader` | 下载到字符串 / 流式读取 |
| `Exists` / `Delete` | 存在性检查 / 删除 |
| `GetMeta` / `SetMeta` | 读 / 改写元数据（S3 下走「自我复制」，部分后端不支持时仅告警） |
| `Copy` / `ListObjects` | 同桶复制 / 前缀列举（分页） |

### 高级操作 (`advanced/`)

| 方法 | 面试重点 |
|------|---------|
| `MultipartUpload` | CreateMultipartUpload → UploadPart × N → Complete 三阶段 |
| `ConcurrentMultipartUpload` | goroutine + 信号量控制并发；**合并前按 part number 升序**（已修复乱序 bug） |
| `ResumableUpload` | checkpoint 文件记录 uploadID + 各片 ETag，中断可续 |
| `ResumableDownload` | HTTP Range 断点续传下载 |

### 签名 URL (`presign/`)

| 方法 | 说明 |
|------|------|
| `GetPresignedURL` / `PutPresignedURL` | 临时下载 / 客户端直传链接 |
| `UploadWithSignedURL` / `DownloadWithSignedURL` | 用签名 URL 真实走一遍上传 / 下载 |

### 实战场景 (`examples/`)

| 场景 | 核心技巧 |
|------|---------|
| 大数据流式导出 | 先落临时文件再上传（规避 io.Pipe 的 Content-Length 难题） |
| 临时下载链接 | 签名 URL 封装成 `GetDownloadURL` |
| 生命周期规则 | 过期删除 / 转 IA / 转 Archive（OSS 仅支持 `private`/`public-read`/`public-read-write` 等子集） |

## 为什么不用阿里云专有 SDK（`aliyun-oss-go-sdk`）？

专有 SDK 的 `bucket.SignURL()`、`oss.ResponseContentType()` 等 API 在 S3 生态里没有对应物，
一旦用了就**锁死在阿里云**，本地 MinIO 完全跑不了。统一到 S3 接口后：
本地用 MinIO 验证、生产用 OSS 的 S3 兼容模式，代码零改动。

（若要用到 OSS 图片处理 `x-oss-process` 等独有能力，则需在业务层对 OSS 后端做特判。）

## 面试自测

1. 分片上传三阶段？第二阶段某片失败怎么处理（AbortMultipartUpload 清理残留）？
2. 断点续传的 checkpoint 存了什么（uploadID + 各片 ETag）？凭什么能续？
3. 并发分片上传合并时为什么必须按 part number 升序？
4. 为什么本仓库刻意不用 `manager.Uploader`？（aws-chunked 与 OSS 不兼容）
5. 签名 URL 安全原理？为什么不能让客户端知道 AccessKey？
6. path-style 与 virtual-hosted 的区别？为什么 OSS 强制 virtual-hosted？
