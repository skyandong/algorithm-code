package basic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"oss-lab/storage"
)

// ObjectInfo 对象信息（与后端无关的自定义结构）
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ETag         string
}

// Client 封装对象存储的基础操作
//
// 所有方法对本地 MinIO、阿里云 OSS、AWS S3 行为一致。
// 注意：本包不再 import 任何厂商 SDK，只依赖 storage.Client 的领域方法。
type Client struct {
	sc *storage.Client
}

// New 创建基础操作客户端
func New(c *storage.Client) *Client {
	return &Client{sc: c}
}

// ======================== 对象上传 ========================

// PutFromString 上传字符串内容
func (c *Client) PutFromString(ctx context.Context, objectKey string, content string) error {
	data := []byte(content)
	if _, err := c.sc.Put(ctx, storage.PutInput{
		Bucket:        c.sc.Bucket(),
		Key:           objectKey,
		Body:          bytes.NewReader(data),
		ContentLength: int64(len(data)),
	}); err != nil {
		return fmt.Errorf("上传字符串到 [%s] 失败: %w", objectKey, err)
	}
	fmt.Printf("✅ 上传成功: %s (%d bytes)\n", objectKey, len(content))
	return nil
}

// PutFromFile 上传本地文件
func (c *Client) PutFromFile(ctx context.Context, objectKey, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	if _, err := c.sc.Put(ctx, storage.PutInput{
		Bucket: c.sc.Bucket(),
		Key:    objectKey,
		Body:   f,
	}); err != nil {
		return fmt.Errorf("上传 [%s] -> [%s] 失败: %w", filePath, objectKey, err)
	}

	info, _ := os.Stat(filePath)
	fmt.Printf("✅ 上传成功: %s -> %s (%d bytes)\n", filePath, objectKey, info.Size())
	return nil
}

// PutFromReader 从 io.Reader 上传（最通用，适合流式场景）
func (c *Client) PutFromReader(ctx context.Context, objectKey string, reader io.Reader) error {
	if _, err := c.sc.Put(ctx, storage.PutInput{
		Bucket: c.sc.Bucket(),
		Key:    objectKey,
		Body:   reader,
	}); err != nil {
		return fmt.Errorf("上传到 [%s] 失败: %w", objectKey, err)
	}
	fmt.Printf("✅ 上传成功: %s\n", objectKey)
	return nil
}

// PutWithOptions 上传并指定 Content-Type 与自定义元数据
func (c *Client) PutWithOptions(ctx context.Context, objectKey string, reader io.Reader, contentType string, meta map[string]string) error {
	if _, err := c.sc.Put(ctx, storage.PutInput{
		Bucket:      c.sc.Bucket(),
		Key:         objectKey,
		Body:        reader,
		ContentType: contentType,
		Metadata:    meta,
	}); err != nil {
		return fmt.Errorf("上传 [%s] 失败: %w", objectKey, err)
	}
	fmt.Printf("✅ 上传成功: %s (Content-Type=%s, %d 项元数据)\n", objectKey, contentType, len(meta))
	return nil
}

// ======================== 对象下载 ========================

// GetToString 下载对象内容到字符串
func (c *Client) GetToString(ctx context.Context, objectKey string) (string, error) {
	out, err := c.sc.Get(ctx, storage.GetInput{Bucket: c.sc.Bucket(), Key: objectKey})
	if err != nil {
		return "", fmt.Errorf("下载 [%s] 失败: %w", objectKey, err)
	}
	defer out.Body.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, out.Body); err != nil {
		return "", fmt.Errorf("读取 [%s] 内容失败: %w", objectKey, err)
	}
	return buf.String(), nil
}

// GetToFile 下载对象到本地文件
func (c *Client) GetToFile(ctx context.Context, objectKey, localPath string) error {
	out, err := c.sc.Get(ctx, storage.GetInput{Bucket: c.sc.Bucket(), Key: objectKey})
	if err != nil {
		return fmt.Errorf("下载 [%s] 失败: %w", objectKey, err)
	}
	defer out.Body.Close()

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, out.Body)
	if err != nil {
		return fmt.Errorf("写入本地文件失败: %w", err)
	}

	fmt.Printf("✅ 下载成功: %s -> %s (%d bytes)\n", objectKey, localPath, n)
	return nil
}

// GetReader 获取对象 Reader（流式读取，适合大文件）
//
// 调用方必须负责关闭返回的 ReadCloser。
func (c *Client) GetReader(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	out, err := c.sc.Get(ctx, storage.GetInput{Bucket: c.sc.Bucket(), Key: objectKey})
	if err != nil {
		return nil, fmt.Errorf("获取 [%s] Reader 失败: %w", objectKey, err)
	}
	return out.Body, nil
}

// ======================== 对象管理 ========================

// Delete 删除对象
func (c *Client) Delete(ctx context.Context, objectKey string) error {
	if err := c.sc.Delete(ctx, c.sc.Bucket(), objectKey); err != nil {
		return fmt.Errorf("删除 [%s] 失败: %w", objectKey, err)
	}
	fmt.Printf("🗑️  删除成功: %s\n", objectKey)
	return nil
}

// Exists 检查对象是否存在
func (c *Client) Exists(ctx context.Context, objectKey string) (bool, error) {
	head, err := c.sc.Head(ctx, storage.HeadInput{Bucket: c.sc.Bucket(), Key: objectKey})
	if err != nil {
		return false, fmt.Errorf("检查 [%s] 存在性失败: %w", objectKey, err)
	}
	return head.Exists, nil
}

// ListObjects 列举对象（支持前缀过滤与数量上限，自动翻页）
func (c *Client) ListObjects(ctx context.Context, prefix string, maxKeys int32) ([]ObjectInfo, error) {
	var (
		objects []ObjectInfo
		token   string
		bucket  = c.sc.Bucket()
	)

	for {
		out, err := c.sc.List(ctx, storage.ListInput{
			Bucket:            bucket,
			Prefix:            prefix,
			MaxKeys:           maxKeys,
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("列举对象失败: %w", err)
		}

		for _, obj := range out.Objects {
			objects = append(objects, ObjectInfo{
				Key:          obj.Key,
				Size:         obj.Size,
				LastModified: obj.LastModified,
				ETag:         obj.ETag,
			})
		}

		if !out.IsTruncated {
			break
		}
		token = out.NextContinuationToken
	}

	fmt.Printf("📋 找到 %d 个对象 (prefix=%q)\n", len(objects), prefix)
	return objects, nil
}

// Copy 复制对象（同 bucket 内）
func (c *Client) Copy(ctx context.Context, srcKey, dstKey string) error {
	bucket := c.sc.Bucket()
	if err := c.sc.Copy(ctx, storage.CopyInput{
		SrcBucket: bucket,
		SrcKey:    srcKey,
		DstBucket: bucket,
		DstKey:    dstKey,
	}); err != nil {
		return fmt.Errorf("复制 [%s] -> [%s] 失败: %w", srcKey, dstKey, err)
	}
	fmt.Printf("✅ 复制成功: %s -> %s\n", srcKey, dstKey)
	return nil
}

// SetMeta 设置对象自定义元数据
//
// 【S3 的坑】S3 协议不支持原地修改元数据，唯一方式是用 Copy 把对象复制回自己
// 并声明 REPLACE。此时若不显式带上原 ContentType，它会被重置成
// application/octet-stream —— 所以这里必须先 Head 一次取回。
// 这也意味着大对象改元数据会产生一次完整的数据拷贝。
func (c *Client) SetMeta(ctx context.Context, objectKey string, meta map[string]string) error {
	bucket := c.sc.Bucket()

	head, err := c.sc.Head(ctx, storage.HeadInput{Bucket: bucket, Key: objectKey})
	if err != nil {
		return fmt.Errorf("读取 [%s] 现有属性失败: %w", objectKey, err)
	}

	if err := c.sc.Copy(ctx, storage.CopyInput{
		SrcBucket:         bucket,
		SrcKey:            objectKey,
		DstBucket:         bucket,
		DstKey:            objectKey,
		Metadata:          meta,
		MetadataDirective: storage.MetadataDirectiveReplace,
		ContentType:       head.ContentType, // 必须显式带回，否则 Content-Type 丢失
	}); err != nil {
		return fmt.Errorf("设置 [%s] 元数据失败: %w", objectKey, err)
	}

	fmt.Printf("✅ 元数据设置成功: %s -> %v\n", objectKey, meta)
	return nil
}

// GetMeta 获取对象自定义元数据（返回值已去掉 x-amz-meta- 前缀）
func (c *Client) GetMeta(ctx context.Context, objectKey string) (map[string]string, error) {
	head, err := c.sc.Head(ctx, storage.HeadInput{Bucket: c.sc.Bucket(), Key: objectKey})
	if err != nil {
		return nil, fmt.Errorf("获取 [%s] 元数据失败: %w", objectKey, err)
	}
	return head.Metadata, nil
}
