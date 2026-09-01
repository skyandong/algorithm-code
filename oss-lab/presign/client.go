package presign

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"oss-lab/storage"
)

// Client 封装预签名 URL 操作
//
// 预签名 URL 是 S3 标准能力，本地 MinIO、阿里云 OSS、AWS S3 全部支持，签出来的
// URL 可直接被任意 HTTP 客户端使用。
//
// 与阿里云专有 SDK 的 bucket.SignURL() 相比，这是同一能力的通用写法：
// 专有 SDK 生成的 URL 只在 OSS 上有效，而这里生成的换后端依然有效。
type Client struct {
	sc *storage.Client
}

// New 创建预签名客户端
func New(c *storage.Client) *Client {
	return &Client{sc: c}
}

// ======================== 签名 URL ========================

// GetPresignedURL 生成带签名的 GET URL（用于临时分享下载）
//
// 场景：
//   - 生成临时下载链接给用户
//   - 私有 bucket 中的文件需要临时访问
func (c *Client) GetPresignedURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	url, err := c.sc.PresignGet(ctx, c.sc.Bucket(), objectKey, expires)
	if err != nil {
		return "", fmt.Errorf("生成签名 GET URL 失败: %w", err)
	}
	fmt.Printf("🔗 签名 GET URL (有效期 %v): %s\n", expires, url)
	return url, nil
}

// PutPresignedURL 生成带签名的 PUT URL（用于让客户端直接上传）
//
// 场景：
//   - 前端/移动端直传对象存储，不经过后端服务器
//   - 减轻服务器带宽压力
//
// 注意：签名时不绑定 Content-Type，客户端用任意 Content-Type 上传均可通过；
// 若需要强制约束，请在签名和上传时使用完全一致的 Content-Type。
func (c *Client) PutPresignedURL(ctx context.Context, objectKey string, expires time.Duration) (string, error) {
	url, err := c.sc.PresignPut(ctx, c.sc.Bucket(), objectKey, expires)
	if err != nil {
		return "", fmt.Errorf("生成签名 PUT URL 失败: %w", err)
	}
	fmt.Printf("📤 签名 PUT URL (有效期 %v): %s\n", expires, url)
	return url, nil
}

// GetPresignedURLWithOptions 生成带响应头覆写的签名 URL
//
// ResponseContentType 会让下载响应强制返回指定的 Content-Type，
// 浏览器据此决定是 inline 展示还是 attachment 下载。
// ResponseContentDisposition 可直接触发下载并指定文件名。
func (c *Client) GetPresignedURLWithOptions(ctx context.Context, objectKey string, expires time.Duration, contentType, filename string) (string, error) {
	url, err := c.sc.PresignGetWithOptions(ctx, c.sc.Bucket(), objectKey, storage.PresignGetOptions{
		Expires:                    expires,
		ResponseContentType:        contentType,
		ResponseContentDisposition: fmt.Sprintf(`attachment; filename="%s"`, filename),
	})
	if err != nil {
		return "", fmt.Errorf("生成签名 URL 失败: %w", err)
	}
	fmt.Printf("🔗 签名 GET URL (Content-Type=%s, 有效期 %v): %s\n", contentType, expires, url)
	return url, nil
}

// ======================== 使用签名 URL ========================

// UploadWithSignedURL 通过 PUT 签名 URL 上传内容
//
// 这是「服务端签名、客户端直传」模式的完整演示：
// 后端只负责签发 URL，数据不经过后端，带宽压力转移给对象存储。
func (c *Client) UploadWithSignedURL(ctx context.Context, objectKey string, data []byte, expires time.Duration) error {
	url, err := c.PutPresignedURL(ctx, objectKey, expires)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.ContentLength = int64(len(data))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 各后端成功码不完全一致：S3/OSS 返回 200，部分实现返回 204
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("上传失败 (status=%d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ 签名 URL 上传成功: %s (%d bytes)\n", objectKey, len(data))
	return nil
}

// DownloadWithSignedURL 通过 GET 签名 URL 下载内容
func (c *Client) DownloadWithSignedURL(ctx context.Context, objectKey string, expires time.Duration) ([]byte, error) {
	url, err := c.GetPresignedURL(ctx, objectKey, expires)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("下载失败 (status=%d): %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	fmt.Printf("✅ 签名 URL 下载成功: %s (%d bytes)\n", objectKey, len(data))
	return data, nil
}
