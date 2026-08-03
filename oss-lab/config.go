package main

import (
	"fmt"
	"os"
)

// OSSConfig 从环境变量读取阿里云 OSS 配置
type OSSConfig struct {
	Endpoint        string // 例如: oss-cn-hangzhou.aliyuncs.com
	AccessKeyID     string
	AccessKeySecret string
	BucketName      string // 默认操作的 Bucket
}

// LoadConfig 从环境变量加载配置
func LoadConfig() (*OSSConfig, error) {
	cfg := &OSSConfig{
		Endpoint:        os.Getenv("OSS_ENDPOINT"),
		AccessKeyID:     os.Getenv("OSS_ACCESS_KEY_ID"),
		AccessKeySecret: os.Getenv("OSS_ACCESS_KEY_SECRET"),
		BucketName:      os.Getenv("OSS_BUCKET_NAME"),
	}

	var missing []string
	if cfg.Endpoint == "" {
		missing = append(missing, "OSS_ENDPOINT")
	}
	if cfg.AccessKeyID == "" {
		missing = append(missing, "OSS_ACCESS_KEY_ID")
	}
	if cfg.AccessKeySecret == "" {
		missing = append(missing, "OSS_ACCESS_KEY_SECRET")
	}
	if cfg.BucketName == "" {
		missing = append(missing, "OSS_BUCKET_NAME")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("缺少环境变量: %v\n请设置:\n  export OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com\n  export OSS_ACCESS_KEY_ID=your-key\n  export OSS_ACCESS_KEY_SECRET=your-secret\n  export OSS_BUCKET_NAME=your-bucket", missing)
	}

	return cfg, nil
}
