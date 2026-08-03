package advanced

import (
	"fmt"
	"os"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Client 封装高级 OSS 操作
type Client struct {
	client *oss.Client
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
		return nil, fmt.Errorf("获取 Bucket [%s] 失败: %w", bucketName, err)
	}

	return &Client{client: client, bucket: bucket}, nil
}

// ======================== 分片上传 ========================

// MultipartUpload 分片上传大文件
// 适用场景: 文件 > 100MB，提高上传效率和可靠性
//
// 流程:
//  1. InitiateMultipartUpload  → 拿到 uploadID
//  2. UploadPart               → 分片上传（可并发）
//  3. CompleteMultipartUpload  → 合并分片
func (c *Client) MultipartUpload(objectKey string, filePath string, partSize int64) error {
	// 打开文件
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	fileInfo, _ := f.Stat()
	fileSize := fileInfo.Size()

	// Step 1: 初始化分片上传
	imur, err := c.bucket.InitiateMultipartUpload(objectKey)
	if err != nil {
		return fmt.Errorf("初始化分片上传失败: %w", err)
	}
	fmt.Printf("📦 分片上传开始: %s (大小: %.2f MB, 分片: %d MB)\n",
		objectKey, float64(fileSize)/(1024*1024), partSize/(1024*1024))

	// Step 2: 逐片上传
	var parts []oss.UploadPart
	chunkNum := int(fileSize / partSize)
	if fileSize%partSize != 0 {
		chunkNum++
	}

	for i := 0; i < chunkNum; i++ {
		start := int64(i) * partSize
		curPartSize := partSize
		if start+partSize > fileSize {
			curPartSize = fileSize - start
		}

		part, err := c.bucket.UploadPart(imur, f, curPartSize, i+1)
		if err != nil {
			// 失败时取消上传
			c.bucket.AbortMultipartUpload(imur)
			return fmt.Errorf("上传第 %d 片失败: %w", i+1, err)
		}
		parts = append(parts, part)
		fmt.Printf("  ▸ 分片 %d/%d 完成 (offset=%d, size=%d)\n", i+1, chunkNum, start, curPartSize)
	}

	// Step 3: 合并分片
	_, err = c.bucket.CompleteMultipartUpload(imur, parts)
	if err != nil {
		c.bucket.AbortMultipartUpload(imur)
		return fmt.Errorf("合并分片失败: %w", err)
	}

	fmt.Printf("✅ 分片上传完成: %s (共 %d 片)\n", objectKey, len(parts))
	return nil
}

// ======================== 断点续传上传 ========================

// ResumableUpload 断点续传上传
// 核心: 使用分片上传 + checkpoint 文件记录进度
//
// 原理:
//  1. 如果 checkpoint 存在 -> 从上次中断的分片继续
//  2. 如果 checkpoint 不存在 -> 从头开始分片上传
//  3. 完成后删除 checkpoint
func (c *Client) ResumableUpload(objectKey string, filePath string, checkpointFile string, partSize int64) error {
	// SDK 内置了断点续传支持，设置 EnableCheckpoint=true 即可
	err := c.bucket.UploadFile(objectKey, filePath, partSize,
		oss.Checkpoint(true, checkpointFile),
		oss.Routines(3), // 3 个并发上传分片
	)
	if err != nil {
		return fmt.Errorf("断点续传上传失败: %w", err)
	}

	fileInfo, _ := os.Stat(filePath)
	fmt.Printf("✅ 断点续传上传成功: %s -> %s (%.2f MB)\n",
		filePath, objectKey, float64(fileInfo.Size())/(1024*1024))
	return nil
}

// ======================== 断点续传下载 ========================

// ResumableDownload 断点续传下载大文件
func (c *Client) ResumableDownload(objectKey string, localPath string, checkpointFile string, partSize int64) error {
	err := c.bucket.DownloadFile(objectKey, localPath, partSize,
		oss.Checkpoint(true, checkpointFile),
		oss.Routines(3),
	)
	if err != nil {
		return fmt.Errorf("断点续传下载失败: %w", err)
	}

	fileInfo, _ := os.Stat(localPath)
	fmt.Printf("✅ 断点续传下载成功: %s -> %s (%.2f MB)\n",
		objectKey, localPath, float64(fileInfo.Size())/(1024*1024))
	return nil
}

// ======================== 并发分片上传（goroutine 加速） ========================

// ConcurrentMultipartUpload 并发分片上传
// 将所有分片并发上传，大幅加速大文件上传
func (c *Client) ConcurrentMultipartUpload(objectKey string, filePath string, partSize int64, concurrency int) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	fileInfo, _ := f.Stat()
	fileSize := fileInfo.Size()

	imur, err := c.bucket.InitiateMultipartUpload(objectKey)
	if err != nil {
		return fmt.Errorf("初始化分片上传失败: %w", err)
	}

	chunkNum := int(fileSize / partSize)
	if fileSize%partSize != 0 {
		chunkNum++
	}

	fmt.Printf("📦 并发分片上传: %s (%d 片, 并发数=%d)\n", objectKey, chunkNum, concurrency)

	// 用 channel 控制并发
	type partResult struct {
		part oss.UploadPart
		num  int
		err  error
	}

	resultCh := make(chan partResult, chunkNum)
	semaphore := make(chan struct{}, concurrency)

	for i := 0; i < chunkNum; i++ {
		go func(chunkIndex int) {
			semaphore <- struct{}{}        // 获取令牌
			defer func() { <-semaphore }() // 释放令牌

			start := int64(chunkIndex) * partSize
			size := partSize
			if start+partSize > fileSize {
				size = fileSize - start
			}

			// 每个 goroutine 需要独立的 reader
			partF, err := os.Open(filePath)
			if err != nil {
				resultCh <- partResult{err: err, num: chunkIndex + 1}
				return
			}
			defer partF.Close()
			partF.Seek(start, 0)

			part, err := c.bucket.UploadPart(imur, ioLimitReader(partF, size), size, chunkIndex+1)
			resultCh <- partResult{part: part, num: chunkIndex + 1, err: err}
		}(i)
	}

	// 收集结果
	parts := make([]oss.UploadPart, chunkNum)
	for i := 0; i < chunkNum; i++ {
		r := <-resultCh
		if r.err != nil {
			c.bucket.AbortMultipartUpload(imur)
			return fmt.Errorf("分片 %d 上传失败: %w", r.num, r.err)
		}
		parts[r.num-1] = r.part
		fmt.Printf("  ▸ 分片 %d/%d 完成\n", r.num, chunkNum)
	}

	_, err = c.bucket.CompleteMultipartUpload(imur, parts)
	if err != nil {
		c.bucket.AbortMultipartUpload(imur)
		return fmt.Errorf("合并分片失败: %w", err)
	}

	fmt.Printf("✅ 并发分片上传完成: %s\n", objectKey)
	return nil
}

// ioLimitReader 限制读取字节数的 Reader
func ioLimitReader(r *os.File, n int64) *ioLimitedReader {
	return &ioLimitedReader{r: r, n: n}
}

type ioLimitedReader struct {
	r *os.File
	n int64
}

func (l *ioLimitedReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, fmt.Errorf("EOF")
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}
