package examples

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ======================== 实战场景 1: 文件存储服务 ========================

// FileService 文件存储服务（典型业务封装）
type FileService struct {
	bucket *oss.Bucket
}

// NewFileService 创建文件服务
func NewFileService(endpoint, accessKeyID, accessKeySecret, bucketName string) (*FileService, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}
	return &FileService{bucket: bucket}, nil
}

// UploadFile 上传文件，按日期分目录，返回 OSS key
// key 格式: uploads/2024-01-15/uuid_filename.pdf
func (s *FileService) UploadFile(fileHeader *multipart.FileHeader) (string, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	// 按日期分目录，防止单个目录文件过多
	dateDir := time.Now().Format("2006-01-02")
	objectKey := fmt.Sprintf("uploads/%s/%d_%s", dateDir, time.Now().UnixNano(), fileHeader.Filename)

	err = s.bucket.PutObject(objectKey, src)
	if err != nil {
		return "", fmt.Errorf("上传文件失败: %w", err)
	}

	return objectKey, nil
}

// GetDownloadURL 生成临时下载链接
func (s *FileService) GetDownloadURL(objectKey string, expireSeconds int64) (string, error) {
	return s.bucket.SignURL(objectKey, oss.HTTPGet, expireSeconds)
}

// ======================== 实战场景 2: 静态网站托管 ========================

// StaticSiteDeployer 静态网站部署器
type StaticSiteDeployer struct {
	bucket *oss.Bucket
}

// NewStaticSiteDeployer 创建部署器
func NewStaticSiteDeployer(endpoint, accessKeyID, accessKeySecret, bucketName string) (*StaticSiteDeployer, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}
	return &StaticSiteDeployer{bucket: bucket}, nil
}

// Deploy 部署本地目录到 OSS，自动设置 Content-Type
func (d *StaticSiteDeployer) Deploy(localDir string) error {
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, _ := filepath.Rel(localDir, path)
		objectKey := filepath.ToSlash(relPath) // 统一用 /

		contentType := detectContentType(relPath)
		err = d.bucket.PutObjectFromFile(objectKey, path, oss.ContentType(contentType))
		if err != nil {
			return fmt.Errorf("上传 [%s] 失败: %w", objectKey, err)
		}

		fmt.Printf("  ✅ %s (%s)\n", objectKey, contentType)
		return nil
	})
}

// detectContentType 根据文件扩展名推断 Content-Type
func detectContentType(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
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
// 场景: 将百万级数据库记录导出为 CSV，上传到 OSS 供下载
type DataExporter struct {
	bucket *oss.Bucket
}

// NewDataExporter 创建导出器
func NewDataExporter(endpoint, accessKeyID, accessKeySecret, bucketName string) (*DataExporter, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}
	return &DataExporter{bucket: bucket}, nil
}

// ExportAndUpload 流式导出并上传
// 关键: 使用 io.Pipe 连接导出和上传，不需要落盘，内存占用可控
func (e *DataExporter) ExportAndUpload(objectKey string, generateCSV func(w io.Writer) error) error {
	reader, writer := io.Pipe()

	// goroutine 1: 生成 CSV 数据
	go func() {
		defer writer.Close()
		if err := generateCSV(writer); err != nil {
			writer.CloseWithError(err)
		}
	}()

	// goroutine 2 (通过 SDK): 流式上传
	err := e.bucket.PutObject(objectKey, reader)
	if err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}

	fmt.Printf("✅ 流式导出上传成功: %s\n", objectKey)
	return nil
}

// ======================== 实战场景 4: 对象生命周期管理 ========================

// LifecycleManager 生命周期管理器
type LifecycleManager struct {
	bucket *oss.Bucket
}

// NewLifecycleManager 创建管理器
func NewLifecycleManager(endpoint, accessKeyID, accessKeySecret, bucketName string) (*LifecycleManager, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}
	return &LifecycleManager{bucket: bucket}, nil
}

// SetLogAutoExpire 设置日志自动过期（通过 SetBucketLifecycle API）
// 场景: 日志文件 30 天后自动删除，节省存储成本
//
// 如果 SDK 版本没有 SetBucketLifecycle，可以通过 REST API 直接调用：
//
//	func (m *LifecycleManager) SetLogAutoExpire(prefix string, expireDays int) error {
//	    lifecycleXML := fmt.Sprintf(`<LifecycleConfiguration>
//	  <Rule><ID>auto-expire</ID><Prefix>%s</Prefix><Status>Enabled</Status>
//	  <Expiration><Days>%d</Days></Expiration></Rule>
//	</LifecycleConfiguration>`, prefix, expireDays)
//	    // 通过 bucket.Client.Conn 发送 PUT ?lifecycle 请求
//	    return nil
//	}
func SetLogAutoExpireExample(prefix string, expireDays int) {
	fmt.Printf("💡 生命周期规则: 对 %s/ 前缀文件设置 %d 天后自动过期\n", prefix, expireDays)
	fmt.Printf("   阿里云控制台: OSS → %s → 数据管理 → 生命周期规则\n", prefix)
	fmt.Printf("   REST API: PUT /?lifecycle\n")
	fmt.Printf("   SDK 低层调用: bucket.Client.Conn 发送签名的 HTTP 请求\n")
}

// SetArchiveRule 设置归档规则
// 场景: 30 天后转为低频存储，90 天后转为归档存储
func SetArchiveRuleExample(prefix string) {
	fmt.Printf("💡 归档规则: %s/ 前缀 30天→IA低频, 90天→Archive归档\n", prefix)
	fmt.Printf("   控制台: OSS → Bucket → 生命周期 → 创建规则\n")
}
