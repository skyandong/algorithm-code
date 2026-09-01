// Package config 负责加载 TRTC 签名服务所需的环境变量配置。
package config

import (
	"os"
	"strconv"
)

// Config 保存 TRTC 签名服务的全部配置。
//
// 所有字段均从环境变量读取，避免把密钥硬编码进代码或提交进 git。
// 生产环境建议通过部署平台的密钥管理能力注入这些环境变量。
type Config struct {
	// SDKAppID 是腾讯云实时音视频(TRTC)控制台中的应用 ID。
	SDKAppID int
	// SDKSecretKey 是对应的签名密钥，控制台「应用信息」中可获取。
	SDKSecretKey string
	// Expire 是生成的票据(UserSig/PrivateMapKey)有效期，单位秒。
	// 默认 7 天。
	Expire int
	// Port 是 HTTP 服务监听端口，默认 8080。
	Port string
	// WebDir 是前端页面目录（相对路径），默认 ./web。
	WebDir string
}

// Load 从环境变量读取配置。缺失的可选字段使用默认值。
func Load() (*Config, error) {
	cfg := &Config{}

	sdkAppID, err := strconv.Atoi(os.Getenv("TRTC_SDKAPPID"))
	if err != nil || sdkAppID == 0 {
		return nil, &ErrMissingEnv{Field: "TRTC_SDKAPPID"}
	}
	cfg.SDKAppID = sdkAppID

	key := os.Getenv("TRTC_SECRETKEY")
	if key == "" {
		return nil, &ErrMissingEnv{Field: "TRTC_SECRETKEY"}
	}
	cfg.SDKSecretKey = key

	// 可选：过期时间，默认 7 天
	expire, err := strconv.Atoi(os.Getenv("TRTC_EXPIRE"))
	if err != nil || expire <= 0 {
		expire = 604800 // 7 * 24 * 3600
	}
	cfg.Expire = expire

	// 可选：监听端口，默认 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	cfg.Port = port

	// 可选：前端页面目录，默认 ./web
	webDir := os.Getenv("TRTC_WEB_DIR")
	if webDir == "" {
		webDir = "./web"
	}
	cfg.WebDir = webDir

	return cfg, nil
}

// ErrMissingEnv 表示缺少必需的环境变量。
type ErrMissingEnv struct{ Field string }

func (e *ErrMissingEnv) Error() string {
	return "缺少必需的环境变量: " + e.Field
}
