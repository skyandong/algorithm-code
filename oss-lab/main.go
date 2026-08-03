package main

import (
	"fmt"
	"os"
	"time"

	"oss-lab/advanced"
	"oss-lab/basic"
	"oss-lab/presign"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}

	fmt.Printf("🚀 OSS Lab 启动\n")
	fmt.Printf("   Endpoint: %s\n", cfg.Endpoint)
	fmt.Printf("   Bucket:   %s\n\n", cfg.BucketName)

	// 初始化三个客户端
	basicCli, err := basic.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret, cfg.BucketName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌ basic client:", err)
		os.Exit(1)
	}

	advCli, err := advanced.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret, cfg.BucketName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌ advanced client:", err)
		os.Exit(1)
	}

	presignCli, err := presign.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret, cfg.BucketName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌ presign client:", err)
		os.Exit(1)
	}
	_ = advCli
	_ = presignCli

	// 运行基础演示
	demoBasic(basicCli)
}

// ======================== 基础操作演示 ========================

func demoBasic(c *basic.Client) {
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  1. 基础操作")
	fmt.Println("═══════════════════════════════════════════")

	// --- 上传字符串 ---
	c.PutFromString("demo/hello.txt", "Hello, 阿里云 OSS!\n这是一段测试内容。")

	// --- 检查是否存在 ---
	exists, _ := c.Exists("demo/hello.txt")
	fmt.Printf("   对象存在: %v\n\n", exists)

	// --- 下载到字符串 ---
	content, err := c.GetToString("demo/hello.txt")
	if err == nil {
		fmt.Printf("   下载内容:\n%s\n", content)
	}

	// --- 元数据 ---
	c.SetMeta("demo/hello.txt", map[string]string{
		"author":  "oss-lab",
		"version": "1.0",
	})
	meta, _ := c.GetMeta("demo/hello.txt")
	fmt.Printf("   元数据: %v\n\n", meta)

	// --- 复制 ---
	c.Copy("demo/hello.txt", "demo/hello-copy.txt")

	// --- 列举 ---
	objects, _ := c.ListObjects("demo/", 10)
	for _, obj := range objects {
		fmt.Printf("   - %s (%d bytes, %s)\n", obj.Key, obj.Size, obj.LastModified.Format(time.RFC3339))
	}

	// --- 签名 URL ---
	fmt.Println()
	demoPresign()

	// --- 清理 ---
	fmt.Println("\n--- 清理测试数据 ---")
	c.Delete("demo/hello.txt")
	c.Delete("demo/hello-copy.txt")
}

// ======================== 签名 URL 演示 ========================

func demoPresign() {
	cfg, _ := LoadConfig()
	cli, err := presign.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret, cfg.BucketName)
	if err != nil {
		fmt.Println("presign:", err)
		return
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  2. 签名 URL（临时授权访问）")
	fmt.Println("═══════════════════════════════════════════")

	url, _ := cli.GetPresignedURL("demo/hello.txt", 10*time.Minute)
	fmt.Printf("   GET 签名URL: %s\n", url)
	url, _ = cli.PutPresignedURL("demo/upload-via-url.txt", 5*time.Minute)
	fmt.Printf("   PUT 签名URL: %s\n", url)
}
