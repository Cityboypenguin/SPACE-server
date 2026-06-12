package systemsettings

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ManageSystemSettingUsecase struct {
    settingRepo repository.SystemSettingRepository
}

func NewManageSystemSettingUsecase(repo repository.SystemSettingRepository) *ManageSystemSettingUsecase {
    return &ManageSystemSettingUsecase{settingRepo: repo}
}

func (u *ManageSystemSettingUsecase) Execute(ctx context.Context, enabled bool) error {
    value := "false"
    if enabled {
        value = "true"
    }

    return u.settingRepo.Update(ctx, "is_report_enabled", value, time.Now().Unix())
}