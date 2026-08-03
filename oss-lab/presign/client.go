package presign

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Client 封装签名 URL 操作
type Client struct {
	bucket *oss.Bucket
}

// New 创建客户端
func New(endpoint, accessKeyID, accessKeySecret, bucketName string) (*Client, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建 OSS client 失败: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("获取 Bucket 失败: %w", err)
	}

	return &Client{bucket: bucket}, nil
}

// ======================== 签名 URL ========================

// GetPresignedURL 生成带签名的 GET URL（用于临时分享下载）
//
// 场景：
//   - 生成临时下载链接给用户
//   - 私有 Bucket 中的文件需要临时访问
func (c *Client) GetPresignedURL(objectKey string, expires time.Duration) (string, error) {
	url, err := c.bucket.SignURL(objectKey, oss.HTTPGet, int64(expires.Seconds()))
	if err != nil {
		return "", fmt.Errorf("生成签名 URL 失败: %w", err)
	}
	fmt.Printf("🔗 签名 GET URL (有效期 %v): %s\n", expires, url)
	return url, nil
}

// PutPresignedURL 生成带签名的 PUT URL（用于让客户端直接上传）
//
// 场景：
//   - 前端/移动端直传 OSS，不经过后端服务器
//   - 减轻服务器带宽压力
func (c *Client) PutPresignedURL(objectKey string, expires time.Duration) (string, error) {
	url, err := c.bucket.SignURL(objectKey, oss.HTTPPut, int64(expires.Seconds()))
	if err != nil {
		return "", fmt.Errorf("生成签名 PUT URL 失败: %w", err)
	}
	fmt.Printf("📤 签名 PUT URL (有效期 %v): %s\n", expires, url)
	return url, nil
}

// ======================== 带限制的签名 URL ========================

// GetPresignedURLWithOptions 生成带 Content-Type 限制的签名 URL
// 下载时浏览器会根据 Content-Type 决定如何处理（inline/attachment）
func (c *Client) GetPresignedURLWithOptions(objectKey string, expires time.Duration, contentType string) (string, error) {
	url, err := c.bucket.SignURL(objectKey, oss.HTTPGet, int64(expires.Seconds()),
		oss.ResponseContentType(contentType),
	)
	if err != nil {
		return "", fmt.Errorf("生成签名 URL 失败: %w", err)
	}
	fmt.Printf("🔗 签名 GET URL (Content-Type=%s): %s\n", contentType, url)
	return url, nil
}

// ======================== 使用签名 URL ========================

// UploadWithSignedURL 通过 PUT 签名 URL 上传内容
func (c *Client) UploadWithSignedURL(objectKey string, data []byte, expires time.Duration) error {
	url, err := c.PutPresignedURL(objectKey, expires)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("上传失败 (status=%d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ 签名 URL 上传成功: %s (%d bytes)\n", objectKey, len(data))
	return nil
}
