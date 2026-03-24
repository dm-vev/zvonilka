package storage

import (
	"context"
	"io"
	"time"
)

// PutObjectOptions configures how a blob is written to object storage.
type PutObjectOptions struct {
	ContentType        string
	ContentDisposition string
	CacheControl       string
	Metadata           map[string]string
}

// BlobObject describes a stored blob or a blob metadata projection.
type BlobObject struct {
	Bucket             string
	Key                string
	ETag               string
	ContentLength      int64
	ContentType        string
	ContentDisposition string
	CacheControl       string
	LastModified       time.Time
	Metadata           map[string]string
	VersionID          string
}

// PresignedRequest describes a time-bound request that a client can replay.
type PresignedRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
	Bucket    string
	ObjectKey string
}

// BlobStore exposes generic blob operations for object-storage backends.
type BlobStore interface {
	Provider
	Bucket() string
	PutObject(ctx context.Context, key string, body io.Reader, size int64, options PutObjectOptions) (BlobObject, error)
	GetObject(ctx context.Context, key string) (io.ReadCloser, BlobObject, error)
	HeadObject(ctx context.Context, key string) (BlobObject, error)
	DeleteObject(ctx context.Context, key string) error
	PresignPutObject(ctx context.Context, key string, expires time.Duration, options PutObjectOptions) (PresignedRequest, error)
	PresignGetObject(ctx context.Context, key string, expires time.Duration) (PresignedRequest, error)
}
