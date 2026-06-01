package announcement

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetAnnouncementUseCase struct {
	announcementRepo repository.AnnouncementRepository
}

func NewGetAnnouncementUseCase(repo repository.AnnouncementRepository) *GetAnnouncementUseCase {
	return &GetAnnouncementUseCase{announcementRepo: repo}
}

func (u *GetAnnouncementUseCase) Execute(ctx context.Context, id int64) (*model.Announcement, error) {
	a, err := u.announcementRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("announcement not found")
	}
	return a, nil
}
