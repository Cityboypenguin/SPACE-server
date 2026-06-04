package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MessageRepository interface {
	SaveMessage(ctx context.Context, m *model.Message) error
	GetMessageByID(ctx context.Context, id int64) (*model.Message, error)
	DeleteMessage(ctx context.Context, id int64) (bool, error)
	ListMessagesByRoomID(ctx context.Context, roomID int64) ([]*model.Message, error)
	UpdateMessage(ctx context.Context, m *model.Message) error
	CountUnreadMessages(ctx context.Context, roomID, userID int64, afterTimestamp int64) (int, error)
}
