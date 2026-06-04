package announcement

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type DeleteAnnouncementUseCase struct {
	announcementRepo repository.AnnouncementRepository
}

func NewDeleteAnnouncementUseCase(repo repository.AnnouncementRepository) *DeleteAnnouncementUseCase {
	return &DeleteAnnouncementUseCase{announcementRepo: repo}
}

func (u *DeleteAnnouncementUseCase) Execute(ctx context.Context, id int64) error {
	a, err := u.announcementRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return errors.New("announcement not found")
	}
	return u.announcementRepo.Delete(ctx, id)
}
