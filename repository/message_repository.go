package repository

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// CourseRoomUnread pairs a course chat room with the caller's unread count in it.
type CourseRoomUnread struct {
	RoomID      int64
	UnreadCount int
}

type MessageRepository interface {
	SaveMessage(ctx context.Context, m *model.Message) error
	// GetMessageByID returns the message, or nil if it does not exist or has been soft-deleted.
	GetMessageByID(ctx context.Context, id int64) (*model.Message, error)
	// SoftDeleteMessage marks the message as deleted by deletedBy. It returns false
	// (with no error) if the message does not exist or was already deleted, so that
	// repeated delete attempts are idempotent and never corrupt deletedBy/deletedAt.
	SoftDeleteMessage(ctx context.Context, id int64, deletedBy int64) (bool, error)
	// beforeID: このID未満を取得, afterID: このIDより大きいを取得, afterTime: この時刻以降を取得
	// 戻り値: messages, hasMoreBefore, hasMoreAfter, error
	ListMessagesByRoomID(ctx context.Context, roomID int64, limit int, beforeID *int64, afterID *int64, afterTime *time.Time) ([]*model.Message, bool, bool, error)
	UpdateMessage(ctx context.Context, m *model.Message) error
	CountUnreadMessages(ctx context.Context, roomID, userID int64, afterTimestamp int64) (int, error)
	CountUnreadMessagesByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]int, error)
	CountUnreadMessagesByRoomType(ctx context.Context, userID int64, roomType string) (int, error)
	CountUnreadMessagesPerMember(ctx context.Context, roomID int64, excludeUserID int64) (map[int64]int, error)
	// CountUnreadByCourseRooms returns the unread count for every course room in
	// userID's timetable for the given semester (room_id 昇順). 授業内チャットは
	// room_users を使わないため、既読位置は room_anonymous_identities から取り、
	// まだ一度も開いていない授業は時間割に登録した時点を起点に数える。
	CountUnreadByCourseRooms(ctx context.Context, userID int64, year int, semester string) ([]*CourseRoomUnread, error)
	GetLastMessagesByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]*model.Message, error)
	// GetMessagesByIDs returns the requested messages keyed by ID. IDs that do not
	// exist or were soft-deleted are simply absent from the map (引用返信の返信先取得用)。
	GetMessagesByIDs(ctx context.Context, ids []int64) (map[int64]*model.Message, error)
}
