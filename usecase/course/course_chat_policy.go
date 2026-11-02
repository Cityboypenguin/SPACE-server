package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const courseChatEnabledKey = "course_chat_enabled"

// EnsureCourseChatEnabled blocks course chat content while keeping timetable APIs available.
func EnsureCourseChatEnabled(ctx context.Context, settingRepo repository.SystemSettingRepository) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}
	if authz.IsAdminRole(claims.Role) {
		return nil
	}
	if settingRepo == nil {
		return nil
	}
	value, err := settingRepo.Get(ctx, courseChatEnabledKey)
	if err != nil {
		return err
	}
	if value == "false" {
		return apperr.Forbidden("授業内チャットはメンテナンス中のため、この操作はできません。")
	}
	return nil
}
