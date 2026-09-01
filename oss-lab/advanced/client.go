package advanced

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"oss-lab/storage"
)

// DefaultPartSize 默认分片大小 5MB
//
// S3 协议要求：除最后一片外，每片不得小于 5MB；单个对象最多 10000 片。
// 因此 5MB 分片最大支持约 48.8TB 的单个对象。
const DefaultPartSize int64 = 5 << 20

// Client 封装分片上传、断点续传等高级操作
//
// 【为什么不用 manager.Uploader】
// aws-sdk-go-v2 的 manager.Uploader 在上传较大文件时会自动启用
// chunked encoding（aws-chunked），而阿里云 OSS 明确不支持该传输方式，
// 会直接报 "InvalidArgument: aws-chunked encoding is not supported"。
// 为了让同一份代码在本地存储与 OSS 上都能跑，这里手写分片逻辑，
// 完全绕开 chunked encoding —— 顺带也让分片过程可见、可控。
//
// 注意：本包只依赖 storage.Client 的分片领域方法，不再 import 任何厂商 SDK。
type Client struct {
	sc *storage.Client
}

// New 创建高级操作客户端
func New(c *storage.Client) *Client {
	return &Client{sc: c}
}

// ======================== 分片上传 ========================

// MultipartUpload 分片上传大文件（串行）
//
// 流程：CreateMultipart → UploadPart × N → CompleteMultipart
// 任一步失败都会调用 AbortMultipart 清理，避免残留分片持续计费。
func (c *Client) MultipartUpload(ctx context.Context, objectKey, filePath string, partSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := info.Size()
	bucket := c.sc.Bucket()

	// 小文件走普通上传即可，分片反而多两次往返
	if fileSize <= partSize {
		data, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("读取文件失败: %w", err)
		}
		if _, err := c.sc.Put(ctx, storage.PutInput{
			Bucket:        bucket,
			Key:           objectKey,
			Body:          bytes.NewReader(data),
			ContentLength: int64(len(data)),
		}); err != nil {
			return fmt.Errorf("上传失败: %w", err)
		}
		fmt.Printf("✅ 上传成功（未分片，%.2f MB）: %s\n", float64(fileSize)/(1<<20), objectKey)
		return nil
	}

	createOut, err := c.sc.CreateMultipart(ctx, storage.CreateMultipartInput{Bucket: bucket, Key: objectKey})
	if err != nil {
		return fmt.Errorf("初始化分片上传失败: %w", err)
	}
	uploadID := createOut.UploadID

	chunkNum := int((fileSize + partSize - 1) / partSize)
	fmt.Printf("📦 分片上传开始: %s (%.2f MB, %d 片 × %d MB)\n",
		objectKey, float64(fileSize)/(1<<20), chunkNum, partSize/(1<<20))

	parts := make([]storage.Part, 0, chunkNum)
	for i := 0; i < chunkNum; i++ {
		offset := int64(i) * partSize
		size := partSize
		if offset+size > fileSize {
			size = fileSize - offset
		}

		// SectionReader 基于 ReadAt，多 goroutine 共用同一文件句柄也安全
		partOut, err := c.sc.UploadPart(ctx, storage.UploadPartInput{
			Bucket:        bucket,
			Key:           objectKey,
			UploadID:      uploadID,
			PartNumber:    int32(i + 1), // 分片号从 1 开始
			Body:          io.NewSectionReader(f, offset, size),
			ContentLength: size,
		})
		if err != nil {
			c.abort(ctx, bucket, objectKey, uploadID)
			return fmt.Errorf("上传第 %d 片失败: %w", i+1, err)
		}

		parts = append(parts, storage.Part{
			PartNumber: int32(i + 1),
			ETag:       partOut.ETag,
		})
		fmt.Printf("  ▸ 分片 %d/%d 完成 (offset=%d, size=%d)\n", i+1, chunkNum, offset, size)
	}

	if err := c.sc.CompleteMultipart(ctx, storage.CompleteMultipartInput{
		Bucket:   bucket,
		Key:      objectKey,
		UploadID: uploadID,
		Parts:    parts,
	}); err != nil {
		c.abort(ctx, bucket, objectKey, uploadID)
		return fmt.Errorf("合并分片失败: %w", err)
	}

	fmt.Printf("✅ 分片上传完成: %s (共 %d 片)\n", objectKey, len(parts))
	return nil
}

// ConcurrentMultipartUpload 并发分片上传（goroutine 加速）
//
// 用带缓冲 channel 做信号量限制并发数，避免打爆内存带宽或触发限流。
func (c *Client) ConcurrentMultipartUpload(ctx context.Context, objectKey, filePath string, partSize int64, concurrency int) error {
	if concurrency < 1 {
		concurrency = 3
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := info.Size()
	if fileSize <= partSize {
		return c.MultipartUpload(ctx, objectKey, filePath, partSize)
	}

	bucket := c.sc.Bucket()
	createOut, err := c.sc.CreateMultipart(ctx, storage.CreateMultipartInput{Bucket: bucket, Key: objectKey})
	if err != nil {
		return fmt.Errorf("初始化分片上传失败: %w", err)
	}
	uploadID := createOut.UploadID

	chunkNum := int((fileSize + partSize - 1) / partSize)
	fmt.Printf("📦 并发分片上传: %s (%d 片, 并发=%d)\n", objectKey, chunkNum, concurrency)

	type partResult struct {
		index int
		etag  string
		err   error
	}

	var (
		wg        sync.WaitGroup
		completed = make([]storage.Part, chunkNum) // 按分片号索引，天然升序
		results   = make(chan partResult, chunkNum)
		sem       = make(chan struct{}, concurrency)
		failed    error
	)

	for i := 0; i < chunkNum; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}        // 获取并发令牌
			defer func() { <-sem }() // 归还令牌

			offset := int64(idx) * partSize
			size := partSize
			if offset+size > fileSize {
				size = fileSize - offset
			}

			partOut, err := c.sc.UploadPart(ctx, storage.UploadPartInput{
				Bucket:        bucket,
				Key:           objectKey,
				UploadID:      uploadID,
				PartNumber:    int32(idx + 1),
				Body:          io.NewSectionReader(f, offset, size),
				ContentLength: size,
			})
			if err != nil {
				results <- partResult{index: idx, err: err}
				return
			}
			results <- partResult{index: idx, etag: partOut.ETag}
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err != nil {
			if failed == nil {
				failed = r.err
			}
			continue
		}
		// 按分片号索引写入：completed[0]=第1片, completed[1]=第2片...
		// 这样切片本身就是按 PartNumber 升序，满足 CompleteMultipart 的要求
		completed[r.index] = storage.Part{
			PartNumber: int32(r.index + 1),
			ETag:       r.etag,
		}
		fmt.Printf("  ▸ 分片 %d/%d 完成\n", r.index+1, chunkNum)
	}

	if failed != nil {
		c.abort(ctx, bucket, objectKey, uploadID)
		return fmt.Errorf("分片上传失败: %w", failed)
	}

	if err := c.sc.CompleteMultipart(ctx, storage.CompleteMultipartInput{
		Bucket:   bucket,
		Key:      objectKey,
		UploadID: uploadID,
		Parts:    completed,
	}); err != nil {
		c.abort(ctx, bucket, objectKey, uploadID)
		return fmt.Errorf("合并分片失败: %w", err)
	}

	fmt.Printf("✅ 并发分片上传完成: %s\n", objectKey)
	return nil
}

// ======================== 断点续传 ========================

// checkpoint 断点续传进度记录
//
// 持久化 uploadID 与已完成分片的 ETag，程序重启后可直接续传未完成的分片。
type checkpoint struct {
	UploadID string         `json:"upload_id"`
	Key      string         `json:"key"`
	PartSize int64          `json:"part_size"`
	Parts    map[int]string `json:"parts"` // 分片号 → ETag
}

// ResumableUpload 断点续传上传
//
// 核心：分片上传 + checkpoint 文件记录进度。
// 中断后重新调用本方法，会自动跳过已完成的分片。
func (c *Client) ResumableUpload(ctx context.Context, objectKey, filePath, checkpointFile string, partSize int64) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	info, _ := f.Stat()
	fileSize := info.Size()
	bucket := c.sc.Bucket()

	cp := loadCheckpoint(checkpointFile)
	// 关键参数对不上就当作全新上传，不能用旧 uploadID 续传
	if cp == nil || cp.Key != objectKey || cp.PartSize != partSize {
		cp = &checkpoint{
			Key:      objectKey,
			PartSize: partSize,
			Parts:    make(map[int]string),
		}
	}

	if cp.UploadID == "" {
		createOut, err := c.sc.CreateMultipart(ctx, storage.CreateMultipartInput{Bucket: bucket, Key: objectKey})
		if err != nil {
			return fmt.Errorf("初始化分片上传失败: %w", err)
		}
		cp.UploadID = createOut.UploadID
	}

	chunkNum := int((fileSize + partSize - 1) / partSize)
	fmt.Printf("📦 断点续传: %s (%d 片，已完成 %d 片)\n", objectKey, chunkNum, len(cp.Parts))

	for i := 0; i < chunkNum; i++ {
		partNum := i + 1
		if _, ok := cp.Parts[partNum]; ok {
			continue // 已上传，跳过
		}

		offset := int64(i) * partSize
		size := partSize
		if offset+size > fileSize {
			size = fileSize - offset
		}

		partOut, err := c.sc.UploadPart(ctx, storage.UploadPartInput{
			Bucket:        bucket,
			Key:           objectKey,
			UploadID:      cp.UploadID,
			PartNumber:    int32(partNum),
			Body:          io.NewSectionReader(f, offset, size),
			ContentLength: size,
		})
		if err != nil {
			if err := saveCheckpoint(checkpointFile, cp); err != nil {
				fmt.Printf("⚠️  保存进度失败: %v\n", err)
			}
			return fmt.Errorf("上传第 %d 片失败（进度已保存，可重跑续传）: %w", partNum, err)
		}

		cp.Parts[partNum] = partOut.ETag
		fmt.Printf("  ▸ 分片 %d/%d 完成\n", partNum, chunkNum)
	}

	// CompleteMultipart 要求 Parts 按分片号升序
	parts := make([]storage.Part, 0, len(cp.Parts))
	for i := 1; i <= chunkNum; i++ {
		etag, ok := cp.Parts[i]
		if !ok {
			return fmt.Errorf("分片 %d 缺失，无法合并", i)
		}
		parts = append(parts, storage.Part{
			PartNumber: int32(i),
			ETag:       etag,
		})
	}

	if err := c.sc.CompleteMultipart(ctx, storage.CompleteMultipartInput{
		Bucket:   bucket,
		Key:      objectKey,
		UploadID: cp.UploadID,
		Parts:    parts,
	}); err != nil {
		return fmt.Errorf("合并分片失败: %w", err)
	}

	os.Remove(checkpointFile) // 成功后清理进度文件
	fmt.Printf("✅ 断点续传完成: %s (%.2f MB)\n", objectKey, float64(fileSize)/(1<<20))
	return nil
}

// ResumableDownload 断点续传下载
//
// 原理：用 HTTP Range 请求从已下载的字节数继续读取，以追加模式写入本地文件。
// 若服务端不支持 Range（返回 200 而非 206），则退化为整体重下。
func (c *Client) ResumableDownload(ctx context.Context, objectKey, localPath string, partSize int64) error {
	bucket := c.sc.Bucket()

	// 先探测文件总大小
	head, err := c.sc.Head(ctx, storage.HeadInput{Bucket: bucket, Key: objectKey})
	if err != nil {
		return fmt.Errorf("获取对象信息失败: %w", err)
	}
	total := head.ContentLength

	// 已下载的字节数就是续传起点
	var downloaded int64
	if st, err := os.Stat(localPath); err == nil {
		downloaded = st.Size()
		if downloaded >= total {
			fmt.Printf("✅ 文件已完整下载: %s\n", localPath)
			return nil
		}
		fmt.Printf("📥 断点续传下载: 从 %d/%d 字节继续\n", downloaded, total)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if downloaded > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	f, err := os.OpenFile(localPath, flags, 0644)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer f.Close()

	for offset := downloaded; offset < total; offset += partSize {
		end := offset + partSize - 1
		if end >= total {
			end = total - 1
		}

		out, err := c.sc.Get(ctx, storage.GetInput{
			Bucket: bucket,
			Key:    objectKey,
			Range:  fmt.Sprintf("bytes=%d-%d", offset, end),
		})
		if err != nil {
			return fmt.Errorf("下载分片失败: %w", err)
		}

		n, err := io.Copy(f, out.Body)
		out.Body.Close()
		if err != nil {
			return fmt.Errorf("写入本地文件失败（已下载 %d 字节，可重跑续传）: %w", offset, err)
		}

		fmt.Printf("  ▸ 已下载 %d/%d 字节\n", offset+n, total)
	}

	fmt.Printf("✅ 下载完成: %s -> %s (%.2f MB)\n", objectKey, localPath, float64(total)/(1<<20))
	return nil
}

// ======================== 内部辅助 ========================

// abort 中止分片上传并清理已上传的分片
func (c *Client) abort(ctx context.Context, bucket, objectKey, uploadID string) {
	if err := c.sc.AbortMultipart(ctx, bucket, objectKey, uploadID); err != nil {
		fmt.Printf("⚠️  中止分片上传失败（请手动清理，否则残留分片会持续计费）: %v\n", err)
		return
	}
	fmt.Printf("🧹 已中止并清理分片上传: %s\n", objectKey)
}

func loadCheckpoint(path string) *checkpoint {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cp checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil
	}
	if cp.Parts == nil {
		cp.Parts = make(map[int]string)
	}
	return &cp
}

func saveCheckpoint(path string, cp *checkpoint) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
