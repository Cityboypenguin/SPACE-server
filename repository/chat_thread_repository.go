package repository

import (
	"context"
	"github.com/Cityboypenguin/SPACE-server/model"
)

type ChatThreadRepository interface {
	HasUnread(ctx context.Context, parentRoomID, viewerID int64) (bool, error)
	AnnouncementID(ctx context.Context, roomID int64) (int64, error)
	Create(ctx context.Context, roomID, creatorID int64) error
	Get(ctx context.Context, roomID, viewerID int64) (*model.ChatThread, error)
	List(ctx context.Context, parentRoomID, viewerID int64, limit, offset int) ([]*model.ChatThread, error)
	Update(ctx context.Context, roomID int64, title string, closed bool) error
}
