package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomUserRepository interface {
	AddUserToRoom(ctx context.Context, roomID, userID int64) error
	RemoveUserFromRoom(ctx context.Context, roomID, userID int64) error
	GetUserIDsByRoomID(ctx context.Context, roomID int64) ([]int64, error)
	ListUsersByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64][]*model.User, error)
	ListDMRoomsByUserID(ctx context.Context, userID int64, limit, offset int) ([]*model.Room, int, error)
	FindDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
	FindOrCreateDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
	GetRoomUserRole(ctx context.Context, roomID, userID int64) (string, error)
	SetRoomUserRole(ctx context.Context, roomID, userID int64, role string) error
	CountRoomUsersByRole(ctx context.Context, roomID int64, role string) (int, error)
	ListRoomMembersWithRoles(ctx context.Context, roomID int64) ([]*model.RoomMember, error)
	// UpdateLastRead は既読位置を進める。lastReadMessageID はそのルームの最新
	// メッセージID（1件も無ければ nil）、readAt は既読にした時刻。
	// 既読位置は巻き戻さない（別端末が先に進めていればそちらを残す）。
	UpdateLastRead(ctx context.Context, roomID, userID int64, lastReadMessageID *int64, readAt int64) error
	// GetLastRead returns nil when userID has never read the room.
	GetLastRead(ctx context.Context, roomID, userID int64) (*ReadPosition, error)
	GetMembersLastReadAt(ctx context.Context, roomID int64) (map[int64]*int64, error)
	// GetLastReadAtByRoomIDs は既読時刻だけをまとめて返す（表示用）。未読数は
	// CountUnreadMessagesByRoomIDs が SQL 側で既読位置を見て数えるので、
	// ここでメッセージIDまで持ち帰る必要は無い。
	GetLastReadAtByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]*int64, error)
	GetMembersLastReadAtByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]map[int64]*int64, error)
}
