package repository

import "context"

// SystemSettingCourseChatEnabled は授業チャットの一括停止の設定キー。
const SystemSettingCourseChatEnabled = "course_chat_enabled"

type SystemSettingRepository interface {
	Get(ctx context.Context, key string) (string, error)
	GetBool(ctx context.Context, key string) (bool, error)
	Update(ctx context.Context, key string, value string, updatedAt int64) error
}
