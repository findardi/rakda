package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/sse"
)

type MinioStorage struct {
	client *minio.Client
	bucket string
	core   *minio.Core
}

func NewMinio(cfg config.MinioConfig) (*MinioStorage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.SslMode,
	})

	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	s := &MinioStorage{
		client: client,
		bucket: cfg.BucketName,
		core: &minio.Core{
			Client: client,
		},
	}

	if err := s.ensureBucket(context.Background()); err != nil {
		return nil, err
	}

	return s, nil
}

func (m *MinioStorage) ensureBucket(ctx context.Context) error {
	exist, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}

	if exist {
		return nil
	}

	if err := m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("make bucket: %w", err)
	}

	return nil
}

func (m *MinioStorage) EnsureEncryption(ctx context.Context) error {
	if err := m.client.SetBucketEncryption(ctx, m.bucket, sse.NewConfigurationSSES3()); err != nil {
		return fmt.Errorf("set bucket encryption: %w", err)
	}

	return nil
}

func (m *MinioStorage) EncryptionStatus(ctx context.Context) (string, error) {
	cfg, err := m.client.GetBucketEncryption(ctx, m.bucket)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "ServerSideEncryptionConfigurationNotFoundError" {
			return "", nil
		}

		return "", fmt.Errorf("get bucket encryption: %w", err)
	}

	for _, rule := range cfg.Rules {
		if algo := rule.Apply.SSEAlgorithm; algo != "" {
			return algo, nil
		}
	}

	return "", nil
}

func (m *MinioStorage) PresignedPut(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := m.client.PresignedPutObject(ctx, m.bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}

	return u.String(), nil
}

func (m *MinioStorage) PresignedGet(ctx context.Context, key, filename string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	reqParams.Set("response-content-disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	u, err := m.client.PresignedGetObject(ctx, m.bucket, key, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}

	return u.String(), nil
}

func (m *MinioStorage) Stat(ctx context.Context, key string) (size int64, contentType string, err error) {
	info, err := m.client.StatObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return 0, "", fmt.Errorf("stat object %s: %w", key, ErrNotFound)
		}

		return 0, "", fmt.Errorf("stat object: %w", err)
	}

	return info.Size, info.ContentType, nil
}

func isNotFound(err error) bool {
	return minio.ToErrorResponse(err).Code == "NoSuchKey"
}

func (m *MinioStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})

	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}

	return obj, nil
}

func (m *MinioStorage) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	opts := minio.GetObjectOptions{}
	if err := opts.SetRange(offset, offset+length-1); err != nil {
		return nil, fmt.Errorf("set range: %w", err)
	}

	obj, err := m.client.GetObject(ctx, m.bucket, key, opts)
	if err != nil {
		return nil, fmt.Errorf("get object range: %w", err)
	}

	return obj, nil
}

func (m *MinioStorage) Delete(ctx context.Context, key string) error {
	if err := m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{}); err != nil && !isNotFound(err) {
		return fmt.Errorf("delete object: %w", err)
	}

	return nil
}

func (m *MinioStorage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if _, err := m.client.PutObject(ctx, m.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	return nil
}

func (m *MinioStorage) InitMultipart(ctx context.Context, key string) (string, error) {
	uploadID, err := m.core.NewMultipartUpload(ctx, m.bucket, key, minio.PutObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("init multipart: %w", err)
	}

	return uploadID, nil
}

func (m *MinioStorage) PresignPart(ctx context.Context, key, uploadID string, partNumber int, expiry time.Duration) (string, error) {
	params := url.Values{}
	params.Set("uploadId", uploadID)
	params.Set("partNumber", strconv.Itoa(partNumber))

	u, err := m.client.Presign(ctx, http.MethodPut, m.bucket, key, expiry, params)
	if err != nil {
		return "", fmt.Errorf("presign part: %w", err)
	}

	return u.String(), nil
}

func (m *MinioStorage) ListParts(ctx context.Context, key, uploadID string) ([]Part, error) {
	out := make([]Part, 0)
	marker := 0

	for {
		res, err := m.core.ListObjectParts(ctx, m.bucket, key, uploadID, marker, 1000)
		if err != nil {
			return nil, fmt.Errorf("list parts: %w", err)
		}

		for _, p := range res.ObjectParts {
			out = append(out, Part{
				PartNumber: p.PartNumber,
				ETag:       p.ETag,
				Size:       p.Size,
			})
		}

		if !res.IsTruncated {
			break
		}

		marker = res.NextPartNumberMarker
	}

	return out, nil
}

func (m *MinioStorage) CompleteMultipart(ctx context.Context, key, uploadID, contentType string, parts []Part) error {
	cps := make([]minio.CompletePart, 0, len(parts))
	for _, p := range parts {
		cps = append(cps, minio.CompletePart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		})
	}

	if _, err := m.core.CompleteMultipartUpload(ctx, m.bucket, key, uploadID, cps, minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return fmt.Errorf("complete multipart: %w", err)
	}

	return nil
}

func (m *MinioStorage) AbortMultipart(ctx context.Context, key, uploadID string) error {
	if err := m.core.AbortMultipartUpload(ctx, m.bucket, key, uploadID); err != nil {
		return fmt.Errorf("abort multipart: %w", err)
	}

	return nil
}

func (m *MinioStorage) DeletePrefix(ctx context.Context, prefix string) error {
	objects := m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for obj := range objects {
		if obj.Err != nil {
			return fmt.Errorf("list objects: %w", obj.Err)
		}

		if err := m.client.RemoveObject(ctx, m.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("delete object %s: %w", obj.Key, err)
		}
	}

	return nil
}

func (m *MinioStorage) DeleteOlderThan(ctx context.Context, prefix string, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	deleted := 0

	objects := m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for obj := range objects {
		if obj.Err != nil {
			return deleted, fmt.Errorf("list objects: %w", obj.Err)
		}

		if obj.LastModified.After(cutoff) {
			continue
		}

		if err := m.client.RemoveObject(ctx, m.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return deleted, fmt.Errorf("delete object %s: %w", obj.Key, err)
		}

		deleted++
	}

	return deleted, nil
}

func (m *MinioStorage) AbortIncompleteUploads(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	aborted := 0

	for upload := range m.client.ListIncompleteUploads(ctx, m.bucket, "", true) {
		if upload.Err != nil {
			return aborted, fmt.Errorf("list incomplete upload: %w", upload.Err)
		}

		if upload.Initiated.After(cutoff) {
			continue
		}

		if err := m.core.AbortMultipartUpload(ctx, m.bucket, upload.Key, upload.UploadID); err != nil {
			return aborted, fmt.Errorf("abort multipart %s: %w", upload.Key, err)
		}

		aborted++
	}

	return aborted, nil
}
