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
	// GetLastReadByRoomIDs は既読位置（メッセージID・時刻）をまとめて返す。
	// 一度も読んでいないルームはキーごと欠ける。
	//
	// 未読数の計算には使わない（CountUnreadMessagesByRoomIDs が SQL 側で既読位置を
	// 見て数える）。それでもメッセージIDまで持ち帰るのは、一覧に並ぶ Room が
	// GraphQL の lastReadMessageID を返すため。1件取得（GetLastRead）と一覧とで
	// 片方だけ null になると、どちらの経路で開いたかで未読ページの起点が変わる。
	GetLastReadByRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]*ReadPosition, error)
	GetMembersLastReadAtByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]map[int64]*int64, error)
}
