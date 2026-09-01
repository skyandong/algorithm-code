package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"oss-lab/advanced"
	"oss-lab/basic"
	"oss-lab/examples"
	"oss-lab/presign"
	"oss-lab/storage"
)

func main() {
	cfg, err := storage.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}

	fmt.Println("🚀 OSS Lab 启动（S3 统一接口）")
	fmt.Println(cfg.Describe())
	fmt.Println()

	ctx := context.Background()

	client, err := storage.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌ 创建客户端失败:", err)
		os.Exit(1)
	}

	// 本地存储会自动建 bucket，开箱即用；
	// 云端账号若无建桶权限，这里会给出明确提示。
	if err := client.EnsureBucket(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	fmt.Println()

	basicCli := basic.New(client)
	presignCli := presign.New(client)

	demoBasic(ctx, basicCli)
	demoPresign(ctx, presignCli, basicCli)
	demoAdvanced(ctx, advanced.New(client))
	demoScenarios(ctx, client)

	fmt.Println("\n✨ 全部演示完成")
}

// ======================== 基础操作演示 ========================

func demoBasic(ctx context.Context, c *basic.Client) {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  1. 基础操作")
	fmt.Println("═══════════════════════════════════════")

	const demoKey = "demo/hello.txt"

	// --- 上传（带 Content-Type 与自定义元数据）---
	if err := c.PutWithOptions(ctx, demoKey,
		strings.NewReader("Hello, S3 统一接口!\n这是一段测试内容。\n"),
		"text/plain; charset=utf-8",
		map[string]string{"author": "oss-lab", "version": "1.0"},
	); err != nil {
		fmt.Println("❌ 上传失败:", err)
		return
	}

	// --- 检查是否存在 ---
	exists, err := c.Exists(ctx, demoKey)
	if err != nil {
		fmt.Println("❌ 检查存在性失败:", err)
		return
	}
	fmt.Printf("   对象存在: %v\n\n", exists)

	// --- 下载 ---
	content, err := c.GetToString(ctx, demoKey)
	if err == nil {
		fmt.Printf("   下载内容:\n%s\n", content)
	}

	// --- 元数据 ---
	meta, err := c.GetMeta(ctx, demoKey)
	if err != nil {
		fmt.Printf("⚠️  读取元数据失败: %v\n\n", err)
	} else {
		fmt.Printf("   元数据: %v\n\n", meta)
	}

	// --- 改写元数据（S3 需复制自身，部分后端不支持，失败仅警告）---
	if err := c.SetMeta(ctx, demoKey, map[string]string{
		"author":  "oss-lab",
		"version": "2.0",
	}); err != nil {
		fmt.Printf("⚠️  改写元数据失败（S3 下需复制自身，部分后端不支持）: %v\n\n", err)
	}

	// --- 复制 ---
	if err := c.Copy(ctx, demoKey, "demo/hello-copy.txt"); err != nil {
		fmt.Printf("⚠️  复制失败: %v\n\n", err)
	}

	// --- 列举 ---
	objects, err := c.ListObjects(ctx, "demo/", 10)
	if err == nil {
		for _, obj := range objects {
			fmt.Printf("   - %s (%d bytes, %s)\n",
				obj.Key, obj.Size, obj.LastModified.Format(time.RFC3339))
		}
	}

	// --- 清理 ---
	fmt.Println("\n--- 清理测试数据 ---")
	c.Delete(ctx, demoKey)
	c.Delete(ctx, "demo/hello-copy.txt")
}

// ======================== 签名 URL 演示 ========================

func demoPresign(ctx context.Context, p *presign.Client, b *basic.Client) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  2. 签名 URL（临时授权访问）")
	fmt.Println("═══════════════════════════════════════")

	const demoKey = "demo/presign.txt"

	// 先放一个对象，才有东西可签名
	if err := b.PutFromString(ctx, demoKey, "通过签名 URL 访问的内容"); err != nil {
		fmt.Println("❌ 准备数据失败:", err)
		return
	}

	if _, err := p.GetPresignedURL(ctx, demoKey, 10*time.Minute); err != nil {
		fmt.Println("❌ 生成 GET 签名失败:", err)
	}
	if _, err := p.PutPresignedURL(ctx, "demo/upload-via-url.txt", 5*time.Minute); err != nil {
		fmt.Println("❌ 生成 PUT 签名失败:", err)
	}

	// 用签名 URL 真实走一遍上传 + 下载
	fmt.Println("\n--- 用签名 URL 实际上传并下载 ---")
	if err := p.UploadWithSignedURL(ctx, "demo/uploaded.txt", []byte("客户端直传内容"), 5*time.Minute); err != nil {
		fmt.Println("⚠️  签名 URL 上传失败:", err)
	}
	if data, err := p.DownloadWithSignedURL(ctx, demoKey, 5*time.Minute); err != nil {
		fmt.Println("⚠️  签名 URL 下载失败:", err)
	} else {
		fmt.Printf("   下载到的内容: %q\n", string(data))
	}

	fmt.Println("\n--- 清理测试数据 ---")
	b.Delete(ctx, demoKey)
	b.Delete(ctx, "demo/uploaded.txt")
}

// ======================== 分片上传演示 ========================

func demoAdvanced(ctx context.Context, a *advanced.Client) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  3. 分片上传与断点续传")
	fmt.Println("═══════════════════════════════════════")

	// 造一个 12MB 的临时文件，刚好触发分片（默认 5MB/片）
	tmpPath, err := makeTempFile(12 << 20)
	if err != nil {
		fmt.Println("⚠️  创建测试文件失败:", err)
		return
	}
	defer os.Remove(tmpPath)

	fmt.Printf("   测试文件: %s (12 MB)\n\n", tmpPath)

	if err := a.MultipartUpload(ctx, "demo/large-file.bin", tmpPath, advanced.DefaultPartSize); err != nil {
		fmt.Println("❌ 分片上传失败:", err)
	}

	fmt.Println()
	if err := a.ConcurrentMultipartUpload(ctx, "demo/large-file-concurrent.bin", tmpPath, advanced.DefaultPartSize, 3); err != nil {
		fmt.Println("❌ 并发分片上传失败:", err)
	}

	fmt.Println()
	fmt.Println("--- 断点续传（checkpoint 记录进度）---")
	cpFile := tmpPath + ".checkpoint"
	if err := a.ResumableUpload(ctx, "demo/resumable.bin", tmpPath, cpFile, advanced.DefaultPartSize); err != nil {
		fmt.Println("❌ 断点续传失败:", err)
	}
	os.Remove(cpFile)
}

// ======================== 实战场景演示 ========================

func demoScenarios(ctx context.Context, client *storage.Client) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  4. 实战场景")
	fmt.Println("═══════════════════════════════════════")

	// 流式导出 CSV
	exporter := examples.NewDataExporter(client)
	fmt.Println("--- 场景: 大数据导出 CSV ---")
	if err := exporter.ExportAndUpload(ctx, "exports/report.csv", func(w io.Writer) error {
		if _, err := io.WriteString(w, "id,name,amount\n"); err != nil {
			return err
		}
		for i := 1; i <= 1000; i++ {
			if _, err := fmt.Fprintf(w, "%d,user-%d,%d\n", i, i, i*10); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		fmt.Println("❌ 导出失败:", err)
	}

	// 生成临时下载链接
	fileSvc := examples.NewFileService(client)
	fmt.Println("\n--- 场景: 生成临时下载链接 ---")
	url, err := fileSvc.GetDownloadURL(ctx, "exports/report.csv", 15*time.Minute)
	if err != nil {
		fmt.Println("❌ 生成链接失败:", err)
	} else {
		fmt.Printf("   15 分钟有效的下载链接:\n   %s\n", truncate(url, 120))
	}

	// 生命周期
	fmt.Println("\n--- 场景: 生命周期规则 ---")
	lm := examples.NewLifecycleManager(client)
	if err := lm.SetRules(ctx, []examples.LifecycleRule{
		{ID: "logs-expire", Prefix: "logs/", ExpirationDays: 30},
		{ID: "archive-cold", Prefix: "archive/", TransitionToIADays: 30, TransitionToArchiveDays: 90},
	}); err != nil {
		fmt.Println("❌ 生命周期配置失败:", err)
	}
}

// ======================== 工具函数 ========================

func makeTempFile(size int) (string, error) {
	f, err := os.CreateTemp("", "osslab-*.bin")
	if err != nil {
		return "", err
	}
	defer f.Close()

	// 用可重复的模式填充，方便校验完整性
	buf := make([]byte, 1024*1024)
	for i := range buf {
		buf[i] = byte(i % 251)
	}
	written := 0
	for written < size {
		n := len(buf)
		if size-written < n {
			n = size - written
		}
		if _, err := f.Write(buf[:n]); err != nil {
			return "", err
		}
		written += n
	}
	return f.Name(), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
