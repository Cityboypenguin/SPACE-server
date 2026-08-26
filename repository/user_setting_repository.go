package repository

import "context"

type UserSettingRepository interface {
	Get(ctx context.Context, userID int64, key string) (value string, found bool, err error)
	Set(ctx context.Context, userID int64, key string, value string, updatedAt int64) error
}
