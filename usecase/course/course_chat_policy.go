package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// EnsureCourseChatEnabled blocks course chat content while keeping timetable APIs available.
func EnsureCourseChatEnabled(ctx context.Context, settingRepo repository.SystemSettingRepository) error {
	stopped, err := courseChatStoppedFor(ctx, settingRepo)
	if err != nil {
		return err
	}
	if stopped {
		return apperr.Forbidden("授業内チャットはメンテナンス中のため、この操作はできません。")
	}
	return nil
}

// courseChatStoppedFor は、呼び出した人にとって授業チャットが一括停止されているか。管理者は止まらない。
func courseChatStoppedFor(ctx context.Context, settingRepo repository.SystemSettingRepository) (bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	if authz.IsAdminRole(claims.Role) || settingRepo == nil {
		return false, nil
	}
	value, err := settingRepo.Get(ctx, repository.SystemSettingCourseChatEnabled)
	if err != nil {
		return false, err
	}
	return value == "false", nil
}
