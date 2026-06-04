package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type AnnouncementRepository interface {
	Save(ctx context.Context, a *model.Announcement) error
	FindByID(ctx context.Context, id int64) (*model.Announcement, error)
	ListAll(ctx context.Context, limit int) ([]*model.Announcement, error)
	ListAllUserIDs(ctx context.Context) ([]int64, error)
	Delete(ctx context.Context, id int64) error
	Update(ctx context.Context, a *model.Announcement) error
}
