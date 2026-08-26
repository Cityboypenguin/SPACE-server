package usersettings

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ManageUserSettingUsecase is the common infrastructure for arbitrary per-user
// key/value preferences (theme, notification toggles, etc.), so each new
// preference only needs a typed GraphQL field on top of this - no new migration,
// repository, or usecase per setting.
type ManageUserSettingUsecase struct {
	settingRepo repository.UserSettingRepository
}

func NewManageUserSettingUsecase(repo repository.UserSettingRepository) *ManageUserSettingUsecase {
	return &ManageUserSettingUsecase{settingRepo: repo}
}

func (u *ManageUserSettingUsecase) Get(ctx context.Context, key string) (string, bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return "", false, err
	}
	return u.settingRepo.Get(ctx, claims.ID, key)
}

func (u *ManageUserSettingUsecase) Set(ctx context.Context, key, value string) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}
	return u.settingRepo.Set(ctx, claims.ID, key, value, time.Now().Unix())
}
