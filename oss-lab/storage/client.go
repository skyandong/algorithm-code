package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Client 是与后端无关的「对象存储抽象层」。
//
// 关键设计：所有原子操作（Put/Get/Delete/List/Copy/Head/Presign/Multipart）
// 都收敛在 storage 包内，用与厂商 SDK 无关的「领域参数」表达意图；底层仍走
// aws-sdk-go-v2 的 S3 客户端，但被严格封装为私有字段（s3Client / presignClient），
// storage 之外的任何包都拿不到它，因此也就无法 import 厂商 SDK 或手写 s3.*Input。
//
// 收益：业务包（basic/presign/advanced/examples）只依赖 storage.Client 的领域
// 方法，切换后端（本地 MinIO / 阿里云 OSS / AWS S3）只需改 storage 一处配置，
// 业务代码零改动。这正是「抽象」与上一版「S3 协议兼容」的本质区别——
// 上一版的下游仍直接构造 s3.*Input，换非 S3 后端时那些调用要重写；
// 这一版下游根本碰不到 s3 类型。
type Client struct {
	s3Client      *s3.Client
	presignClient *s3.PresignClient
	cfg           *Config
}

// New 创建客户端
func New(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
		config.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
	}
	// 仅当显式提供了 AK/SK 才使用静态凭证；否则交给 SDK 默认凭证链
	// （环境变量 / ~/.aws/credentials / 实例角色 IAM 等），让 aws 后端在
	// ECS、EKS 等环境下可用 IAM 角色，无需硬编码密钥。
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("加载 AWS 配置失败: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}

		// 寻址风格 —— 切换后端时最致命的一项：
		//   本地 MinIO            → path-style（bucket 在 URL 路径里）
		//   阿里云 OSS            → virtual-hosted（bucket 在域名里，path-style 会被拒）
		o.UsePathStyle = cfg.UsePathStyle

		// 【兼容性关键】自 aws-sdk-go-v2 v1.42 起，SDK 默认对 PutObject / UploadPart
		// 等接口计算并附带 CRC32 校验头。AWS S3 完整支持，但阿里云 OSS、SeaweedFS
		// 等第三方实现支持不完整，轻则忽略、重则返回 400。降级为「仅在协议要求时
		// 计算」，换取最大兼容性。确认后端支持 checksum 时可改回 WhenSupported。
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	return &Client{
		s3Client:      client,
		presignClient: s3.NewPresignClient(client),
		cfg:           cfg,
	}, nil
}

// ======================== 对象读写 ========================

// PutInput 上传对象的领域参数（与厂商 SDK 无关）
type PutInput struct {
	Bucket        string
	Key           string
	Body          io.Reader
	ContentType   string
	ContentLength int64 // 可选；分片/流式场景建议显式传入，避免 SDK 缓冲整段流
	Metadata      map[string]string
}

// PutOutput 上传结果
type PutOutput struct {
	ETag      string
	VersionID string
}

// Put 上传一个对象
func (c *Client) Put(ctx context.Context, in PutInput) (*PutOutput, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(in.Bucket),
		Key:    aws.String(in.Key),
		Body:   in.Body,
	}
	if in.ContentType != "" {
		input.ContentType = aws.String(in.ContentType)
	}
	if in.ContentLength > 0 {
		input.ContentLength = aws.Int64(in.ContentLength)
	}
	if len(in.Metadata) > 0 {
		input.Metadata = in.Metadata
	}

	out, err := c.s3Client.PutObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("上传 [%s/%s] 失败: %w", in.Bucket, in.Key, err)
	}
	return &PutOutput{
		ETag:      aws.ToString(out.ETag),
		VersionID: aws.ToString(out.VersionId),
	}, nil
}

// GetInput 下载对象的领域参数
type GetInput struct {
	Bucket string
	Key    string
	Range  string // 可选，HTTP Range，如 "bytes=0-1023"
}

// GetOutput 下载结果（Body 必须调用方关闭）
type GetOutput struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	ETag          string
	Metadata      map[string]string
	LastModified  time.Time
}

// Get 下载一个对象
func (c *Client) Get(ctx context.Context, in GetInput) (*GetOutput, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(in.Bucket),
		Key:    aws.String(in.Key),
	}
	if in.Range != "" {
		input.Range = aws.String(in.Range)
	}

	out, err := c.s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("下载 [%s/%s] 失败: %w", in.Bucket, in.Key, err)
	}

	res := &GetOutput{
		Body:          out.Body,
		ContentType:   aws.ToString(out.ContentType),
		ContentLength: aws.ToInt64(out.ContentLength),
		ETag:          aws.ToString(out.ETag),
		Metadata:      out.Metadata,
	}
	if out.LastModified != nil {
		res.LastModified = *out.LastModified
	}
	return res, nil
}

// HeadInput 查询对象元数据的领域参数
type HeadInput struct {
	Bucket string
	Key    string
}

// HeadOutput 对象元数据查询结果
type HeadOutput struct {
	Bucket        string
	Key           string
	ContentType   string
	ContentLength int64
	ETag          string
	Metadata      map[string]string
	LastModified  time.Time
	Exists        bool
}

// Head 查询对象元数据；对象不存在时返回 Exists=false，不报错
func (c *Client) Head(ctx context.Context, in HeadInput) (*HeadOutput, error) {
	out, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(in.Bucket),
		Key:    aws.String(in.Key),
	})
	if err != nil {
		if IsNotFound(err) {
			return &HeadOutput{Exists: false, Bucket: in.Bucket, Key: in.Key}, nil
		}
		return nil, fmt.Errorf("查询 [%s/%s] 元数据失败: %w", in.Bucket, in.Key, err)
	}

	res := &HeadOutput{
		Bucket:        in.Bucket,
		Key:           in.Key,
		ContentType:   aws.ToString(out.ContentType),
		ContentLength: aws.ToInt64(out.ContentLength),
		ETag:          aws.ToString(out.ETag),
		Metadata:      out.Metadata,
		Exists:        true,
	}
	if out.LastModified != nil {
		res.LastModified = *out.LastModified
	}
	return res, nil
}

// Delete 删除一个对象
func (c *Client) Delete(ctx context.Context, bucket, key string) error {
	if _, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("删除 [%s/%s] 失败: %w", bucket, key, err)
	}
	return nil
}

// ListInput 列举对象的领域参数
type ListInput struct {
	Bucket            string
	Prefix            string
	Delimiter         string
	MaxKeys           int32
	ContinuationToken string // 翻页令牌，首次留空
}

// ListedObject 列举返回的单条对象信息
type ListedObject struct {
	Key          string
	ETag         string
	Size         int64
	LastModified time.Time
}

// ListOutput 列举结果（单页）
type ListOutput struct {
	Objects               []ListedObject
	IsTruncated           bool
	NextContinuationToken string
}

// List 列举对象（单页）。翻页由调用方用 NextContinuationToken 循环触发。
func (c *Client) List(ctx context.Context, in ListInput) (*ListOutput, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(in.Bucket),
	}
	if in.Prefix != "" {
		input.Prefix = aws.String(in.Prefix)
	}
	if in.Delimiter != "" {
		input.Delimiter = aws.String(in.Delimiter)
	}
	if in.MaxKeys > 0 {
		input.MaxKeys = aws.Int32(in.MaxKeys)
	}
	if in.ContinuationToken != "" {
		input.ContinuationToken = aws.String(in.ContinuationToken)
	}

	out, err := c.s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("列举 [%s] 对象失败: %w", in.Bucket, err)
	}

	objects := make([]ListedObject, 0, len(out.Contents))
	for _, obj := range out.Contents {
		lo := ListedObject{
			Key:  aws.ToString(obj.Key),
			ETag: aws.ToString(obj.ETag),
			Size: aws.ToInt64(obj.Size),
		}
		if obj.LastModified != nil {
			lo.LastModified = *obj.LastModified
		}
		objects = append(objects, lo)
	}

	return &ListOutput{
		Objects:               objects,
		IsTruncated:           aws.ToBool(out.IsTruncated),
		NextContinuationToken: aws.ToString(out.NextContinuationToken),
	}, nil
}

// 元数据指令常量（Copy 时使用）
const (
	MetadataDirectiveCopy    = "COPY"
	MetadataDirectiveReplace = "REPLACE"
)

// CopyInput 复制对象的领域参数
type CopyInput struct {
	SrcBucket         string
	SrcKey            string
	DstBucket         string
	DstKey            string
	Metadata          map[string]string
	MetadataDirective string // MetadataDirectiveCopy / MetadataDirectiveReplace，留空默认 COPY
	ContentType       string // 替换元数据时建议带上原 Content-Type，避免被重置
}

// Copy 复制对象（支持同/跨 bucket）
func (c *Client) Copy(ctx context.Context, in CopyInput) error {
	input := &s3.CopyObjectInput{
		Bucket:     aws.String(in.DstBucket),
		Key:        aws.String(in.DstKey),
		CopySource: aws.String(encodeCopySource(in.SrcBucket, in.SrcKey)),
	}
	if in.ContentType != "" {
		input.ContentType = aws.String(in.ContentType)
	}
	if len(in.Metadata) > 0 {
		input.Metadata = in.Metadata
	}
	if in.MetadataDirective != "" {
		input.MetadataDirective = s3types.MetadataDirective(in.MetadataDirective)
	}

	if _, err := c.s3Client.CopyObject(ctx, input); err != nil {
		return fmt.Errorf("复制 [%s/%s] -> [%s/%s] 失败: %w",
			in.SrcBucket, in.SrcKey, in.DstBucket, in.DstKey, err)
	}
	return nil
}

// encodeCopySource 构造 CopyObject 所需的 CopySource 值
//
// key 中的空格、中文等字符必须转义，否则请求会被拒绝。
func encodeCopySource(bucket, key string) string {
	return url.PathEscape(bucket) + "/" + url.PathEscape(key)
}

// ======================== 分片上传 ========================

// CreateMultipartInput 初始化分片上传的参数
type CreateMultipartInput struct {
	Bucket      string
	Key         string
	ContentType string
	Metadata    map[string]string
}

// CreateMultipartOutput 初始化分片上传的结果
type CreateMultipartOutput struct {
	UploadID string
}

// CreateMultipart 初始化一次分片上传，返回 uploadID
func (c *Client) CreateMultipart(ctx context.Context, in CreateMultipartInput) (*CreateMultipartOutput, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(in.Bucket),
		Key:    aws.String(in.Key),
	}
	if in.ContentType != "" {
		input.ContentType = aws.String(in.ContentType)
	}
	if len(in.Metadata) > 0 {
		input.Metadata = in.Metadata
	}

	out, err := c.s3Client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("初始化分片上传 [%s/%s] 失败: %w", in.Bucket, in.Key, err)
	}
	return &CreateMultipartOutput{UploadID: aws.ToString(out.UploadId)}, nil
}

// UploadPartInput 上传单个分片的参数
type UploadPartInput struct {
	Bucket        string
	Key           string
	UploadID      string
	PartNumber    int32
	Body          io.Reader
	ContentLength int64
}

// UploadPartOutput 上传单个分片的结果
type UploadPartOutput struct {
	ETag string
}

// UploadPart 上传单个分片，返回该分片的 ETag
func (c *Client) UploadPart(ctx context.Context, in UploadPartInput) (*UploadPartOutput, error) {
	input := &s3.UploadPartInput{
		Bucket:     aws.String(in.Bucket),
		Key:        aws.String(in.Key),
		UploadId:   aws.String(in.UploadID),
		PartNumber: aws.Int32(in.PartNumber),
		Body:       in.Body,
	}
	if in.ContentLength > 0 {
		input.ContentLength = aws.Int64(in.ContentLength)
	}

	out, err := c.s3Client.UploadPart(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("上传分片 %d 失败: %w", in.PartNumber, err)
	}
	return &UploadPartOutput{ETag: aws.ToString(out.ETag)}, nil
}

// Part 已上传分片的标识（分片号 + ETag）
type Part struct {
	PartNumber int32
	ETag       string
}

// CompleteMultipartInput 合并分片的参数
//
// 注意：Parts 必须按 PartNumber 升序排列，否则 OSS / MinIO 都会报
// InvalidPartOrder。调用方负责排序（参考 advanced 包的并发分片实现）。
type CompleteMultipartInput struct {
	Bucket   string
	Key      string
	UploadID string
	Parts    []Part
}

// CompleteMultipart 合并分片，完成一次分片上传
func (c *Client) CompleteMultipart(ctx context.Context, in CompleteMultipartInput) error {
	parts := make([]s3types.CompletedPart, 0, len(in.Parts))
	for _, p := range in.Parts {
		parts = append(parts, s3types.CompletedPart{
			ETag:       aws.String(p.ETag),
			PartNumber: aws.Int32(p.PartNumber),
		})
	}

	if _, err := c.s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(in.Bucket),
		Key:      aws.String(in.Key),
		UploadId: aws.String(in.UploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: parts,
		},
	}); err != nil {
		return fmt.Errorf("合并分片 [%s/%s] 失败: %w", in.Bucket, in.Key, err)
	}
	return nil
}

// AbortMultipart 中止分片上传并清理已上传的分片
func (c *Client) AbortMultipart(ctx context.Context, bucket, key, uploadID string) error {
	if _, err := c.s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}); err != nil {
		return fmt.Errorf("中止分片上传 [%s/%s] 失败: %w", bucket, key, err)
	}
	return nil
}

// ListParts 列出已上传的分片，用于断点续传时核对进度
func (c *Client) ListParts(ctx context.Context, bucket, key, uploadID string) ([]Part, error) {
	out, err := c.s3Client.ListParts(ctx, &s3.ListPartsInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		return nil, fmt.Errorf("列举已上传分片失败: %w", err)
	}

	parts := make([]Part, 0, len(out.Parts))
	for _, p := range out.Parts {
		parts = append(parts, Part{
			PartNumber: aws.ToInt32(p.PartNumber),
			ETag:       aws.ToString(p.ETag),
		})
	}
	return parts, nil
}

// ======================== 预签名 URL ========================

// PresignGet 生成带签名的 GET URL（临时分享下载）
func (c *Client) PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error) {
	req, err := c.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("生成签名 GET URL 失败: %w", err)
	}
	return req.URL, nil
}

// PresignPut 生成带签名的 PUT URL（客户端直传）
func (c *Client) PresignPut(ctx context.Context, bucket, key string, expires time.Duration) (string, error) {
	req, err := c.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("生成签名 PUT URL 失败: %w", err)
	}
	return req.URL, nil
}

// PresignUploadPart 生成单个分片的签名 URL（分片直传场景）
func (c *Client) PresignUploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
	req, err := c.presignClient.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("生成分片签名 URL 失败: %w", err)
	}
	return req.URL, nil
}

// PresignGetOptions 带响应头覆写的 GET 签名 URL 选项
type PresignGetOptions struct {
	Expires                    time.Duration
	ResponseContentType        string
	ResponseContentDisposition string
}

// PresignGetWithOptions 生成带响应头覆写的签名 GET URL
//
// ResponseContentType 会让下载响应强制返回指定 Content-Type；
// ResponseContentDisposition 可直接触发下载并指定文件名。
func (c *Client) PresignGetWithOptions(ctx context.Context, bucket, key string, opts PresignGetOptions) (string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if opts.ResponseContentType != "" {
		input.ResponseContentType = aws.String(opts.ResponseContentType)
	}
	if opts.ResponseContentDisposition != "" {
		input.ResponseContentDisposition = aws.String(opts.ResponseContentDisposition)
	}

	exp := opts.Expires
	if exp <= 0 {
		exp = 15 * time.Minute
	}
	req, err := c.presignClient.PresignGetObject(ctx, input, s3.WithPresignExpires(exp))
	if err != nil {
		return "", fmt.Errorf("生成签名 URL 失败: %w", err)
	}
	return req.URL, nil
}

// ======================== 运维辅助 ========================

// Bucket 返回当前操作的 bucket 名
func (c *Client) Bucket() string { return c.cfg.Bucket }

// Config 返回当前生效的配置
func (c *Client) Config() *Config { return c.cfg }

// EnsureBucket 确保 bucket 存在，不存在则尝试创建
//
// 本地 MinIO 会自动创建，开箱即用；阿里云 OSS 需要 RAM 账号具备 oss:PutBucket
// 权限，若无权限则给出提示而非中断。
func (c *Client) EnsureBucket(ctx context.Context) error {
	bucket := c.cfg.Bucket

	_, err := c.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return nil
	}

	if _, err := c.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		return fmt.Errorf(
			"bucket [%s] 不存在且自动创建失败: %w\n"+
				"  请手动创建后重试，或确认账号具备写入权限（阿里云 OSS 需要 oss:PutBucket）",
			bucket, err)
	}

	fmt.Printf("✅ bucket [%s] 创建成功\n", bucket)
	return nil
}

// IsNotFound 判断错误是否为「对象或 bucket 不存在」
//
// 各后端的错误码并不统一（AWS 返回 NotFound，部分实现返回 NoSuchKey 或
// NoSuchBucket），因此统一按 HTTP 404 状态码判断，跨后端都可靠。
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) {
		return respErr.HTTPStatusCode() == http.StatusNotFound
	}
	return false
}

// ListBuckets 列举当前账号下所有 bucket
func (c *Client) ListBuckets(ctx context.Context) ([]string, error) {
	out, err := c.s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("列举 bucket 失败: %w", err)
	}
	names := make([]string, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		names = append(names, aws.ToString(b.Name))
	}
	return names, nil
}
