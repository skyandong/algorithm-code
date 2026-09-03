package examples

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"oss-lab/storage"
)

// ======================== 实战场景 1: 文件存储服务 ========================

// FileService 文件存储服务（典型业务封装）
//
// 这是业务层最该有的样子：只依赖 storage.Client，
// 完全不知道底层是本地 MinIO 还是阿里云 OSS，也不接触任何厂商 SDK。
type FileService struct {
	sc *storage.Client
}

// NewFileService 创建文件服务
func NewFileService(c *storage.Client) *FileService {
	return &FileService{sc: c}
}

// UploadFile 上传文件，按日期分目录，返回对象 key
//
// key 格式: uploads/2026-09-01/1693545600_photo.jpg
// 按日期分目录是为了避免单个前缀下对象过多导致列举变慢。
func (s *FileService) UploadFile(ctx context.Context, fileHeader *multipart.FileHeader) (string, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	dateDir := time.Now().Format("2006-01-02")
	objectKey := fmt.Sprintf("uploads/%s/%d_%s", dateDir, time.Now().UnixNano(), fileHeader.Filename)

	if _, err := s.sc.Put(ctx, storage.PutInput{
		Bucket:      s.sc.Bucket(),
		Key:         objectKey,
		Body:        src,
		ContentType: fileHeader.Header.Get("Content-Type"),
	}); err != nil {
		return "", fmt.Errorf("上传文件失败: %w", err)
	}

	return objectKey, nil
}

// GetDownloadURL 生成临时下载链接
func (s *FileService) GetDownloadURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	url, err := s.sc.PresignGet(ctx, s.sc.Bucket(), objectKey, expires)
	if err != nil {
		return "", fmt.Errorf("生成下载链接失败: %w", err)
	}
	return url, nil
}

// ======================== 实战场景 2: 静态网站托管 ========================

// StaticSiteDeployer 静态网站部署器
type StaticSiteDeployer struct {
	sc *storage.Client
}

// NewStaticSiteDeployer 创建部署器
func NewStaticSiteDeployer(c *storage.Client) *StaticSiteDeployer {
	return &StaticSiteDeployer{sc: c}
}

// Deploy 部署本地目录，自动推断并设置 Content-Type
//
// Content-Type 必须显式设置：S3 默认存为 application/octet-stream，
// 浏览器打开 HTML 会变成下载而不是渲染。
func (d *StaticSiteDeployer) Deploy(ctx context.Context, localDir, prefix string) error {
	count := 0
	bucket := d.sc.Bucket()

	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(localDir, path)
		objectKey := filepath.ToSlash(filepath.Join(prefix, relPath))

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		contentType := detectContentType(relPath)
		if _, err := d.sc.Put(ctx, storage.PutInput{
			Bucket:      bucket,
			Key:         objectKey,
			Body:        f,
			ContentType: contentType,
		}); err != nil {
			return fmt.Errorf("上传 [%s] 失败: %w", objectKey, err)
		}

		count++
		fmt.Printf("  ✅ %s (%s)\n", objectKey, contentType)
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("🎉 部署完成，共 %d 个文件\n", count)
	return nil
}

// detectContentType 根据文件扩展名推断 Content-Type
func detectContentType(filename string) string {
	switch filepath.Ext(filename) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

// ======================== 实战场景 3: 大数据导出 ========================

// DataExporter 大数据导出器
//
// 场景: 将百万级数据库记录导出为 CSV 并上传，供用户下载
type DataExporter struct {
	sc *storage.Client
}

// NewDataExporter 创建导出器
func NewDataExporter(c *storage.Client) *DataExporter {
	return &DataExporter{sc: c}
}

// ExportAndUpload 生成 CSV 并上传
//
// 【为什么不用 io.Pipe 直连上传】
// S3 的 PutObject 是单次 HTTP 请求，必须在请求头带上 Content-Length。
// io.Pipe 的 Reader 既无法预知长度、也不支持 Seek，
// 直接交给 SDK 的结果是：SDK 要么把整个流缓冲进内存（数据量大时直接 OOM），
// 要么因拿不到长度而失败。
//
// 工程上稳妥的做法是先落临时文件再上传：
// 内存占用可控、支持超大文件、失败了还能保留现场。
// 若确实需要「边生成边传」，请使用 advanced.MultipartUpload 的分片方式。
func (e *DataExporter) ExportAndUpload(ctx context.Context, objectKey string, generateCSV func(w io.Writer) error) error {
	tmp, err := os.CreateTemp("", "export-*.csv")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := generateCSV(tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("生成 CSV 失败: %w", err)
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		tmp.Close()
		return err
	}
	info, _ := tmp.Stat()
	size := info.Size()

	if _, err := e.sc.Put(ctx, storage.PutInput{
		Bucket:        e.sc.Bucket(),
		Key:           objectKey,
		Body:          tmp,
		ContentType:   "text/csv; charset=utf-8",
		ContentLength: size,
	}); err != nil {
		tmp.Close()
		return fmt.Errorf("上传失败: %w", err)
	}
	tmp.Close()

	fmt.Printf("✅ 导出上传成功: %s\n", objectKey)
	return nil
}

// ======================== 实战场景 4: 对象生命周期管理 ========================

// LifecycleManager 生命周期管理器
//
// 场景: 日志文件 30 天后自动删除 / 转低频存储，节省存储成本
type LifecycleManager struct {
	sc *storage.Client
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager(c *storage.Client) *LifecycleManager {
	return &LifecycleManager{sc: c}
}

// LifecycleRule 生命周期规则（简化版，只覆盖最常用的过期与转储）
type LifecycleRule struct {
	ID                      string
	Prefix                  string
	ExpirationDays          int32 // >0 表示多少天后删除
	TransitionToIADays      int32 // >0 表示多少天后转低频存储
	TransitionToArchiveDays int32 // >0 表示多少天后转归档存储
}

// SetRules 设置生命周期规则
//
// 【跨后端兼容性现实】
// 生命周期是各厂商实现差异最大的能力之一：AWS S3 / MinIO 相对完整，
// 阿里云 OSS 的 S3 兼容接口不支持 Rule/Filter/And 组合条件（会报 0014-00000006），
// SeaweedFS 社区版支持有限。且 MinIO 只有 STANDARD 存储类，转 IA/Archive 会失败。
//
// 因此生产上建议：生命周期规则走各云控制台或专有 SDK 单独配置，
// 不要放进需要跨云复用的业务代码里。本演示只打印配置意图，不真正下发。
func (m *LifecycleManager) SetRules(ctx context.Context, rules []LifecycleRule) error {
	_ = ctx // 真实下发需各后端专属 API，此处仅演示意图
	fmt.Printf("💡 生命周期规则配置: %d 条\n", len(rules))
	for _, r := range rules {
		fmt.Printf("   - [%s] prefix=%q", r.ID, r.Prefix)
		if r.ExpirationDays > 0 {
			fmt.Printf(" 过期删除=%d天", r.ExpirationDays)
		}
		if r.TransitionToIADays > 0 {
			fmt.Printf(" 转低频=%d天", r.TransitionToIADays)
		}
		if r.TransitionToArchiveDays > 0 {
			fmt.Printf(" 转归档=%d天", r.TransitionToArchiveDays)
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Println("⚠️  各后端对该 API 的兼容程度差异较大：")
	fmt.Println("   · AWS S3 / MinIO   完整支持，可直接用 PutBucketLifecycleConfiguration")
	fmt.Println("   · 阿里云 OSS        仅支持按前缀过滤，不支持 <And> 组合条件")
	fmt.Println("   · SeaweedFS         社区版对生命周期支持有限")
	fmt.Println()
	fmt.Println("   生产环境建议：跨后端场景下，生命周期规则走控制台或各厂商 API 单独配置，")
	fmt.Println("   不要放进需要跨云复用的业务代码里。")
	return nil
}

// SetLogAutoExpireExample 日志自动过期示例（打印配置指引）
func SetLogAutoExpireExample(prefix string, expireDays int) {
	fmt.Printf("💡 生命周期规则: 对 %s/ 前缀文件设置 %d 天后自动过期\n", prefix, expireDays)
	fmt.Printf("   阿里云控制台: OSS → Bucket → 数据管理 → 生命周期规则\n")
	fmt.Printf("   S3 兼容 API:  PUT /?lifecycle\n")
}
