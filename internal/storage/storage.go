package storage

import (
	"context"
	"io"
)

type Object struct {
	Bucket      string
	Key         string
	ContentType string
	SizeBytes   int64
	Checksum    string
}

type Store interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (Object, error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}
