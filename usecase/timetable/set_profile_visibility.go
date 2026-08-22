package timetable

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SetTimetableProfileVisibilityUseCase interface {
	Execute(ctx context.Context, id int64, visible bool) (*model.Timetable, error)
}

var _ SetTimetableProfileVisibilityUseCase = &SetTimetableProfileVisibilityInteractor{}

type SetTimetableProfileVisibilityInteractor struct {
	timetableRepo repository.TimetableRepository
}

func NewSetTimetableProfileVisibilityUseCase(timetableRepo repository.TimetableRepository) SetTimetableProfileVisibilityUseCase {
	return &SetTimetableProfileVisibilityInteractor{timetableRepo: timetableRepo}
}

func (uc *SetTimetableProfileVisibilityInteractor) Execute(ctx context.Context, id int64, visible bool) (*model.Timetable, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return uc.timetableRepo.SetProfileVisibility(ctx, id, claims.ID, visible)
}
