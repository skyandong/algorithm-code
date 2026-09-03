// Package storage 提供一套与后端无关的 S3 兼容对象存储访问层。
//
// 同一份业务代码，仅通过环境变量即可在本地 MinIO / 阿里云 OSS / AWS S3
// 之间切换，业务层无感知。
package storage

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Backend 存储后端类型
type Backend string

const (
	BackendLocal  Backend = "local"  // 本地 MinIO（docker-compose 默认）
	BackendMinIO  Backend = "minio"  // 本地 MinIO
	BackendAliyun Backend = "aliyun" // 阿里云 OSS（S3 兼容模式）
	BackendAWS    Backend = "aws"    // AWS S3
	BackendCustom Backend = "custom" // 完全自定义
)

// Config 统一的存储配置
//
// 两个最容易踩坑的字段：
//
//  1. Endpoint —— 阿里云 OSS 有两套端点，别混用：
//     专有 SDK 用  oss-cn-hangzhou.aliyuncs.com
//     S3 SDK 用   s3.oss-cn-hangzhou.aliyuncs.com   ← 本仓库用这个
//     两者不通用，填错会报 SignatureDoesNotMatch。
//
//  2. UsePathStyle —— 寻址风格：
//     path-style      http://127.0.0.1:9000/mybucket/key  （bucket 在路径里）
//     virtual-hosted  https://mybucket.s3.oss-cn-hangzhou.aliyuncs.com/key
//     阿里云 OSS 只支持 virtual-hosted style，path-style 请求会被直接拒绝；
//     而 MinIO 本地部署一般用 path-style。
type Config struct {
	Backend      Backend
	Endpoint     string
	Region       string
	AccessKey    string
	SecretKey    string
	Bucket       string
	UsePathStyle bool
}

// preset 后端预设：只需少量环境变量即可完成配置
type preset struct {
	endpoint     string // 空表示用 AWS 默认端点（仅 aws 后端）
	region       string
	usePathStyle bool
	// 本地后端的默认凭证，方便 docker-compose 起来就能直接跑
	defaultAK string
	defaultSK string
}

var presets = map[Backend]preset{
	BackendLocal: {
		endpoint:     "http://127.0.0.1:9000", // MinIO S3 网关端口
		region:       "us-east-1",             // 本地存储不校验 region，任意值均可
		usePathStyle: true,
		defaultAK:    "minioadmin",
		defaultSK:    "minioadmin",
	},
	BackendMinIO: {
		endpoint:     "http://127.0.0.1:9000",
		region:       "us-east-1",
		usePathStyle: true,
		defaultAK:    "minioadmin",
		defaultSK:    "minioadmin",
	},
	BackendAliyun: {
		region:       "cn-hangzhou",
		usePathStyle: false, // 关键：OSS 拒绝 path-style
	},
	BackendAWS: {
		region:       "us-east-1",
		usePathStyle: false,
	},
	BackendCustom: {
		region:       "us-east-1",
		usePathStyle: true,
	},
}

// LoadConfig 从环境变量加载配置
//
// 环境变量（STORAGE_* 优先，不存在时回退到旧版 OSS_*）：
//
//	STORAGE_BACKEND      local | minio | aliyun | aws | custom（默认 local）
//	STORAGE_ENDPOINT     覆盖预设端点
//	STORAGE_REGION       region（aliyun 后端默认 cn-hangzhou）
//	STORAGE_ACCESS_KEY   访问密钥 ID
//	STORAGE_SECRET_KEY   访问密钥 Secret
//	STORAGE_BUCKET       bucket 名称
//	STORAGE_PATH_STYLE   true|false，覆盖预设的寻址风格
func LoadConfig() (*Config, error) {
	backend := Backend(strings.ToLower(env("STORAGE_BACKEND", "local")))

	p, ok := presets[backend]
	if !ok {
		return nil, fmt.Errorf("未知的 STORAGE_BACKEND: %q（可选 local/minio/aliyun/aws/custom）", backend)
	}

	cfg := &Config{
		Backend:      backend,
		Endpoint:     p.endpoint,
		Region:       p.region,
		AccessKey:    env("STORAGE_ACCESS_KEY", env("OSS_ACCESS_KEY_ID", p.defaultAK)),
		SecretKey:    env("STORAGE_SECRET_KEY", env("OSS_ACCESS_KEY_SECRET", p.defaultSK)),
		Bucket:       env("STORAGE_BUCKET", env("OSS_BUCKET_NAME", "oss-lab")),
		UsePathStyle: p.usePathStyle,
	}

	// 阿里云 OSS 的端点依赖 region，region 可能被覆盖，所以先定 region 再拼端点
	if region := env("STORAGE_REGION", ""); region != "" {
		cfg.Region = region
	}
	if backend == BackendAliyun {
		cfg.Endpoint = fmt.Sprintf("https://s3.oss-%s.aliyuncs.com", cfg.Region)
	}

	// 显式指定的端点优先级最高（内网端点、CNAME 自定义域名都靠它）
	if ep := env("STORAGE_ENDPOINT", env("OSS_ENDPOINT", "")); ep != "" {
		cfg.Endpoint = ep
	}

	if ps := env("STORAGE_PATH_STYLE", ""); ps != "" {
		v, err := strconv.ParseBool(ps)
		if err != nil {
			return nil, fmt.Errorf("STORAGE_PATH_STYLE 解析失败（应为 true/false）: %w", err)
		}
		cfg.UsePathStyle = v
	}

	// 校验：除 aws 后端走 SDK 默认端点外，其余都必须有端点
	var missing []string
	if cfg.Endpoint == "" && backend != BackendAWS {
		missing = append(missing, "STORAGE_ENDPOINT")
	}
	if cfg.AccessKey == "" {
		missing = append(missing, "STORAGE_ACCESS_KEY")
	}
	if cfg.SecretKey == "" {
		missing = append(missing, "STORAGE_SECRET_KEY")
	}
	if cfg.Bucket == "" {
		missing = append(missing, "STORAGE_BUCKET")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("缺少环境变量: %v\n\n%s", missing, UsageHint(backend))
	}

	return cfg, nil
}

// UsageHint 按后端给出可直接复制的配置示例
func UsageHint(b Backend) string {
	switch b {
	case BackendLocal:
		return `本地 MinIO（先 docker compose up -d）:
  export STORAGE_BACKEND=local
  export STORAGE_BUCKET=oss-lab
  # 凭证默认 minioadmin/minioadmin，与 docker-compose.yml 保持一致`
	case BackendAliyun:
		return `阿里云 OSS（S3 兼容模式）:
  export STORAGE_BACKEND=aliyun
  export STORAGE_REGION=cn-hangzhou
  export STORAGE_ACCESS_KEY=<RAM 子账号 AccessKey ID>
  export STORAGE_SECRET_KEY=<RAM 子账号 AccessKey Secret>
  export STORAGE_BUCKET=<bucket 名>

注意: 2025-03-20 起新开通的 OSS，中国内地 Bucket 不能用默认外网域名
      调用上传/下载等数据类 API，需配置 CNAME 自定义域名后，再用
      STORAGE_ENDPOINT=https://<你的自定义域名> 覆盖。`
	case BackendAWS:
		return `AWS S3:
  export STORAGE_BACKEND=aws
  export STORAGE_REGION=us-east-1
  export STORAGE_ACCESS_KEY=<AK>
  export STORAGE_SECRET_KEY=<SK>
  export STORAGE_BUCKET=<bucket 名>`
	default:
		return `自定义 S3 兼容存储:
  export STORAGE_BACKEND=custom
  export STORAGE_ENDPOINT=http://host:port
  export STORAGE_ACCESS_KEY=<ak>
  export STORAGE_SECRET_KEY=<sk>
  export STORAGE_BUCKET=<bucket>
  export STORAGE_PATH_STYLE=true   # 本地自建一般 true；阿里云 OSS 必须 false`
	}
}

// Describe 打印当前生效的配置（密钥打码）
func (c *Config) Describe() string {
	ak := c.AccessKey
	if len(ak) > 6 {
		ak = ak[:3] + "***" + ak[len(ak)-3:]
	}
	style := "virtual-hosted"
	if c.UsePathStyle {
		style = "path-style"
	}
	return fmt.Sprintf(
		"  后端:      %s\n  端点:      %s\n  Region:    %s\n  Bucket:    %s\n  AccessKey: %s\n  寻址风格:  %s",
		c.Backend, c.Endpoint, c.Region, c.Bucket, ak, style,
	)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
