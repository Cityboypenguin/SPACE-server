package announcement

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateAnnouncementInput struct {
	ID    int64
	Title string
	Body  string
}

type UpdateAnnouncementUseCase struct {
	announcementRepo repository.AnnouncementRepository
}

func NewUpdateAnnouncementUseCase(repo repository.AnnouncementRepository) *UpdateAnnouncementUseCase {
	return &UpdateAnnouncementUseCase{announcementRepo: repo}
}

func (u *UpdateAnnouncementUseCase) Execute(ctx context.Context, input UpdateAnnouncementInput) (*model.Announcement, error) {
	a, err := u.announcementRepo.FindByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("announcement not found")
	}

	title := strings.TrimSpace(input.Title)
	body := strings.TrimSpace(input.Body)

	if title == "" || body == "" {
		return nil, errors.New("title and body are required")
	}
	if len(title) > 255 {
		return nil, errors.New("title must be 255 characters or less")
	}

	a.Title = title
	a.Body = body
	a.UpdatedAt = time.Now()

	if err := u.announcementRepo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}
