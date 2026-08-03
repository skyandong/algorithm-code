package basic

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Client 封装 OSS 基础操作
type Client struct {
	client *oss.Client
	bucket *oss.Bucket
}

// New 创建 OSS 客户端并获取 Bucket
func New(endpoint, accessKeyID, accessKeySecret, bucketName string) (*Client, error) {
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建 OSS client 失败: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("获取 Bucket [%s] 失败: %w", bucketName, err)
	}

	return &Client{client: client, bucket: bucket}, nil
}

// ======================== Bucket 操作 ========================

// ListBuckets 列举所有 Bucket
func (c *Client) ListBuckets() ([]oss.BucketProperties, error) {
	result, err := c.client.ListBuckets()
	if err != nil {
		return nil, fmt.Errorf("列举 Bucket 失败: %w", err)
	}
	return result.Buckets, nil
}

// CreateBucket 创建 Bucket（ACL 默认私有）
func (c *Client) CreateBucket(bucketName string, acl oss.ACLType) error {
	err := c.client.CreateBucket(bucketName, oss.ACL(acl))
	if err != nil {
		return fmt.Errorf("创建 Bucket [%s] 失败: %w", bucketName, err)
	}
	fmt.Printf("✅ Bucket [%s] 创建成功，ACL=%s\n", bucketName, acl)
	return nil
}

// ======================== 对象上传 ========================

// PutFromString 上传字符串内容
func (c *Client) PutFromString(objectKey string, content string) error {
	err := c.bucket.PutObject(objectKey, strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("上传字符串到 [%s] 失败: %w", objectKey, err)
	}
	fmt.Printf("✅ 上传成功: %s (%d bytes)\n", objectKey, len(content))
	return nil
}

// PutFromFile 上传本地文件
func (c *Client) PutFromFile(objectKey string, filePath string) error {
	err := c.bucket.PutObjectFromFile(objectKey, filePath)
	if err != nil {
		return fmt.Errorf("上传文件 [%s] -> [%s] 失败: %w", filePath, objectKey, err)
	}
	info, _ := os.Stat(filePath)
	fmt.Printf("✅ 上传成功: %s -> %s (%d bytes)\n", filePath, objectKey, info.Size())
	return nil
}

// PutFromReader 从 io.Reader 上传（最通用）
func (c *Client) PutFromReader(objectKey string, reader io.Reader) error {
	err := c.bucket.PutObject(objectKey, reader)
	if err != nil {
		return fmt.Errorf("上传到 [%s] 失败: %w", objectKey, err)
	}
	fmt.Printf("✅ 上传成功: %s\n", objectKey)
	return nil
}

// ======================== 对象下载 ========================

// GetToString 下载对象内容到字符串
func (c *Client) GetToString(objectKey string) (string, error) {
	body, err := c.bucket.GetObject(objectKey)
	if err != nil {
		return "", fmt.Errorf("下载 [%s] 失败: %w", objectKey, err)
	}
	defer body.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, body); err != nil {
		return "", fmt.Errorf("读取 [%s] 内容失败: %w", objectKey, err)
	}
	return buf.String(), nil
}

// GetToFile 下载对象到本地文件
func (c *Client) GetToFile(objectKey string, localPath string) error {
	err := c.bucket.GetObjectToFile(objectKey, localPath)
	if err != nil {
		return fmt.Errorf("下载 [%s] -> [%s] 失败: %w", objectKey, localPath, err)
	}
	info, _ := os.Stat(localPath)
	fmt.Printf("✅ 下载成功: %s -> %s (%d bytes)\n", objectKey, localPath, info.Size())
	return nil
}

// GetReader 获取对象 Reader（流式读取，适合大文件）
func (c *Client) GetReader(objectKey string) (io.ReadCloser, error) {
	body, err := c.bucket.GetObject(objectKey)
	if err != nil {
		return nil, fmt.Errorf("获取 [%s] Reader 失败: %w", objectKey, err)
	}
	return body, nil
}

// ======================== 对象管理 ========================

// Delete 删除对象
func (c *Client) Delete(objectKey string) error {
	err := c.bucket.DeleteObject(objectKey)
	if err != nil {
		return fmt.Errorf("删除 [%s] 失败: %w", objectKey, err)
	}
	fmt.Printf("🗑️  删除成功: %s\n", objectKey)
	return nil
}

// Exists 检查对象是否存在
func (c *Client) Exists(objectKey string) (bool, error) {
	exists, err := c.bucket.IsObjectExist(objectKey)
	if err != nil {
		return false, fmt.Errorf("检查 [%s] 存在性失败: %w", objectKey, err)
	}
	return exists, nil
}

// ListObjects 列举对象（支持前缀、分页）
func (c *Client) ListObjects(prefix string, maxKeys int) ([]oss.ObjectProperties, error) {
	marker := ""
	var allObjects []oss.ObjectProperties

	for {
		lsRes, err := c.bucket.ListObjects(
			oss.Prefix(prefix),
			oss.Marker(marker),
			oss.MaxKeys(maxKeys),
		)
		if err != nil {
			return nil, fmt.Errorf("列举对象失败: %w", err)
		}

		allObjects = append(allObjects, lsRes.Objects...)

		if !lsRes.IsTruncated {
			break
		}
		marker = lsRes.NextMarker
	}

	fmt.Printf("📋 找到 %d 个对象 (prefix=%q)\n", len(allObjects), prefix)
	return allObjects, nil
}

// Copy 复制对象（同 Bucket 内）
func (c *Client) Copy(srcKey, dstKey string) error {
	_, err := c.bucket.CopyObject(srcKey, dstKey)
	if err != nil {
		return fmt.Errorf("复制 [%s] -> [%s] 失败: %w", srcKey, dstKey, err)
	}
	fmt.Printf("✅ 复制成功: %s -> %s\n", srcKey, dstKey)
	return nil
}

// SetMeta 设置对象元数据
func (c *Client) SetMeta(objectKey string, meta map[string]string) error {
	var options []oss.Option
	for k, v := range meta {
		options = append(options, oss.Meta(k, v))
	}
	err := c.bucket.SetObjectMeta(objectKey, options...)
	if err != nil {
		return fmt.Errorf("设置 [%s] 元数据失败: %w", objectKey, err)
	}
	fmt.Printf("✅ 元数据设置成功: %s -> %v\n", objectKey, meta)
	return nil
}

// GetMeta 获取对象元数据
func (c *Client) GetMeta(objectKey string) (map[string]string, error) {
	props, err := c.bucket.GetObjectDetailedMeta(objectKey)
	if err != nil {
		return nil, fmt.Errorf("获取 [%s] 元数据失败: %w", objectKey, err)
	}

	meta := make(map[string]string)
	for k, v := range props {
		if strings.HasPrefix(k, "X-Oss-Meta-") {
			meta[strings.TrimPrefix(k, "X-Oss-Meta-")] = v[0]
		}
	}
	return meta, nil
}
