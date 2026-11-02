package systemsettings

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ManageSystemSettingUsecase struct {
	settingRepo repository.SystemSettingRepository
}

const courseChatEnabledKey = "course_chat_enabled"

func NewManageSystemSettingUsecase(repo repository.SystemSettingRepository) *ManageSystemSettingUsecase {
	return &ManageSystemSettingUsecase{settingRepo: repo}
}

func (u *ManageSystemSettingUsecase) Execute(ctx context.Context, enabled bool) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	value := "false"
	if enabled {
		value = "true"
	}

	return u.settingRepo.Update(ctx, "is_report_enabled", value, time.Now().Unix())
}

func (u *ManageSystemSettingUsecase) IsReportEnabled(ctx context.Context) (bool, error) {
	return u.settingRepo.GetBool(ctx, "is_report_enabled")
}

func (u *ManageSystemSettingUsecase) SetCourseChatEnabled(ctx context.Context, enabled bool) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	value := "false"
	if enabled {
		value = "true"
	}
	return u.settingRepo.Update(ctx, courseChatEnabledKey, value, time.Now().Unix())
}

func (u *ManageSystemSettingUsecase) IsCourseChatEnabled(ctx context.Context) (bool, error) {
	value, err := u.settingRepo.Get(ctx, courseChatEnabledKey)
	if err != nil {
		return false, err
	}
	if value == "" {
		return true, nil
	}
	return value == "true", nil
}
