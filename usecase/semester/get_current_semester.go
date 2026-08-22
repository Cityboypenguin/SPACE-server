package semester

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetCurrentSemesterUseCase interface {
	Execute(ctx context.Context) (year int, semesterName string, err error)
}

var _ GetCurrentSemesterUseCase = &GetCurrentSemesterInteractor{}

type GetCurrentSemesterInteractor struct {
	settingRepo repository.SystemSettingRepository
}

func NewGetCurrentSemesterUseCase(settingRepo repository.SystemSettingRepository) GetCurrentSemesterUseCase {
	return &GetCurrentSemesterInteractor{settingRepo: settingRepo}
}

func (uc *GetCurrentSemesterInteractor) Execute(ctx context.Context) (int, string, error) {
	if _, err := authz.RequireAuth(ctx); err != nil {
		return 0, "", err
	}
	return Get(ctx, uc.settingRepo)
}
