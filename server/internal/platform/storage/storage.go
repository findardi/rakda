package storage

import (
	"context"
	"io"
	"time"
)

type Part struct {
	PartNumber int
	ETag       string
	Size       int64
}
type Storage interface {
	PresignedPut(ctx context.Context, key string, expiry time.Duration) (string, error)
	PresignedGet(ctx context.Context, key, filename string, expiry time.Duration) (string, error)
	Stat(ctx context.Context, key string) (size int64, contentType string, err error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	DeletePrefix(ctx context.Context, prefix string) error

	InitMultipart(ctx context.Context, key string) (string, error)
	PresignPart(ctx context.Context, key, uploadID string, partNumber int, expiry time.Duration) (string, error)
	ListParts(ctx context.Context, key, uploadID string) ([]Part, error)
	CompleteMultipart(ctx context.Context, key, uploadID, contentType string, parts []Part) error
	AbortMultipart(ctx context.Context, key, uploadID string) error
	AbortIncompleteUploads(ctx context.Context, olderThan time.Duration) (int, error)
}
