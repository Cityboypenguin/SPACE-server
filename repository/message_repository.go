package repository

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type MessageRepository interface {
	SaveMessage(ctx context.Context, m *model.Message) error
	GetMessageByID(ctx context.Context, id int64) (*model.Message, error)
	DeleteMessage(ctx context.Context, id int64) (bool, error)
	// beforeID: このID未満を取得, afterID: このIDより大きいを取得, afterTime: この時刻以降を取得
	// 戻り値: messages, hasMoreBefore, hasMoreAfter, error
	ListMessagesByRoomID(ctx context.Context, roomID int64, limit int, beforeID *int64, afterID *int64, afterTime *time.Time) ([]*model.Message, bool, bool, error)
	UpdateMessage(ctx context.Context, m *model.Message) error
	CountUnreadMessages(ctx context.Context, roomID, userID int64, afterTimestamp int64) (int, error)
	CountUnreadMessagesByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int, error)
	CountUnreadMessagesByRoomType(ctx context.Context, userID int64, roomType string) (int, error)
	CountUnreadMessagesPerMember(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error)
	GetLastMessagesByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error)
}
