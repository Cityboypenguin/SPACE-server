package repository

import (
	"context"
	"io"
	"time"
)

type StorageRepository interface {
	// PresignedPutURL generates a presigned PUT URL for direct-to-storage upload.
	// maxBytes is informational for now; enforce server-side limits via bucket policy or presigned POST policy.
	PresignedPutURL(ctx context.Context, objectKey string, contentType string, expires time.Duration, maxBytes int64) (string, error)
	PublicURL(objectKey string) string
	DeleteObject(ctx context.Context, objectKey string) error
}

type PrivateStorageRepository interface {
	PutPrivateObject(ctx context.Context, objectKey, contentType string, body io.Reader, size int64) error
	OpenPrivateObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
	ListPrivateObjects(ctx context.Context, prefix string) ([]PrivateObject, error)
	DeletePrivateObject(ctx context.Context, objectKey string) error
}

type PrivateObject struct {
	Key          string
	LastModified time.Time
}
