package repository

import (
	"context"
	"time"
)

type StorageRepository interface {
	// PresignedPutURL generates a presigned PUT URL for direct-to-storage upload.
	// maxBytes is informational for now; enforce server-side limits via bucket policy or presigned POST policy.
	PresignedPutURL(ctx context.Context, objectKey string, contentType string, expires time.Duration, maxBytes int64) (string, error)
	PublicURL(objectKey string) string
	DeleteObject(ctx context.Context, objectKey string) error
}
