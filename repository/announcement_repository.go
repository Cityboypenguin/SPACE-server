package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type AnnouncementRepository interface {
	Save(ctx context.Context, a *model.Announcement) error
	FindByID(ctx context.Context, id int64) (*model.Announcement, error)
	ListAll(ctx context.Context, q PageQuery) ([]*model.Announcement, int, error)
	Delete(ctx context.Context, id int64) error
	Update(ctx context.Context, a *model.Announcement) error
}
