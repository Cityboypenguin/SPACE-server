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

func (u *ListAnnouncementsUseCase) Execute(ctx context.Context, q repository.PageQuery) ([]*model.Announcement, int, error) {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 50
	}
	return u.announcementRepo.ListAll(ctx, q)
}
