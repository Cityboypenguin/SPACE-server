package announcement

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListAnnouncementsUseCase struct {
	announcementRepo repository.AnnouncementRepository
}

func NewListAnnouncementsUseCase(repo repository.AnnouncementRepository) *ListAnnouncementsUseCase {
	return &ListAnnouncementsUseCase{announcementRepo: repo}
}

func (u *ListAnnouncementsUseCase) Execute(ctx context.Context, limit int) ([]*model.Announcement, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return u.announcementRepo.ListAll(ctx, limit)
}
