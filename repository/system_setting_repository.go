package repository

import "context"

type SystemSettingRepository interface {
    Get(ctx context.Context, key string) (string, error)
    GetBool(ctx context.Context, key string) (bool, error)
    Update(ctx context.Context, key string, value string, updatedAt int64) error
}