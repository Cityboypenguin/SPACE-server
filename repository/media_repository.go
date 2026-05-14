package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MediaRepository interface {
	CreateMedia(ctx context.Context, m *model.Media) error
	CreatePostMedia(ctx context.Context, postID, mediaID int64, position int) error
	CreateMessageMedia(ctx context.Context, messageID, mediaID int64, position int) error
	ListByPostID(ctx context.Context, postID int64) ([]*model.Media, error)
	ListByMessageID(ctx context.Context, messageID int64) ([]*model.Media, error)
}
