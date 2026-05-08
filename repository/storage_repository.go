package repository

import (
	"context"
	"time"
)

type StorageRepository interface {
	PresignedPutURL(ctx context.Context, objectKey string, contentType string, expires time.Duration) (string, error)
	PublicURL(objectKey string) string
	DeleteObject(ctx context.Context, objectKey string) error
}
