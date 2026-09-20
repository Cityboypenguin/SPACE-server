package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomUserRepository interface {
	AddUserToRoom(ctx context.Context, roomID, userID int64) error
	RemoveUserFromRoom(ctx context.Context, roomID, userID int64) error

	// RemoveUsersFromRoom は RemoveUserFromRoom の一括版（IN 句の DELETE 1本）。
	// メンバー編集のようにまとめて外す経路で使う。
	RemoveUsersFromRoom(ctx context.Context, roomID int64, userIDs []int64) error
	GetUserIDsByRoomID(ctx context.Context, roomID int64) ([]int64, error)
	// IsRoomMember は userID が roomID に在籍しているかだけを返す。
	//
	// 権限判定は「自分が入っているか」しか要らないのに、以前は
	// GetUserIDsByRoomID でルーム全員のIDを持ち帰ってから線形探索していた。
	// 閲覧・購読・編集・削除のたびに走る経路なので、1万人のコミュニティでは
	// 1回の判定で1万行を運んでいたことになる。行数に依らない EXISTS 1本にする。
	//
	// 宛先の一覧が要る経路（送信後の配信・既読通知）は引き続き
	// GetUserIDsByRoomID を使う。そちらは全員ぶんが結果そのものなので、
	// この口では置き換えられない。
	IsRoomMember(ctx context.Context, roomID, userID int64) (bool, error)
	ListUsersByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64][]*model.User, error)
	// SearchRoomUsersByPrefix is the bounded display search used by mention
	// suggestions. It does not replace complete membership validation APIs.
	SearchRoomUsersByPrefix(ctx context.Context, roomID int64, prefix string, limit int) ([]*model.User, error)
	// CountUsersByRoomIDs は複数ルームの在籍人数を1クエリで数える。
	//
	// コミュニティ一覧の memberCount 用。以前は一覧の1件ごとに
	// GetUserIDsByRoomID でメンバーIDを全部取り、その len を人数にしていた。
	// 20件のコミュニティで20クエリ、しかも人数しか使わないのに行を全部
	// 持ち帰っていた。
	//
	// 在籍が0のルームは key ごと欠ける。int のゼロ値がそのまま「0人」という
	// 正しい値になるので、呼び出し側は欠けを気にしなくてよい。
	CountUsersByRoomIDs(ctx context.Context, roomIDs []int64) (map[int64]int, error)
	// ListJoinedRoomIDs は roomIDs のうち userID が在籍しているものを返す。
	//
	// コミュニティ一覧の isMember 用。人数と同じ理由で、一覧ぶんを1クエリにする。
	// 在籍していないルームは key ごと欠ける（bool のゼロ値 false が正しい）。
	ListJoinedRoomIDs(ctx context.Context, userID int64, roomIDs []int64) (map[int64]bool, error)
	ListDMRoomsByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Room, int, error)
	FindOrCreateDMRoom(ctx context.Context, userID1, userID2 int64) (*model.Room, error)
	GetRoomUserRole(ctx context.Context, roomID, userID int64) (string, error)
	SetRoomUserRole(ctx context.Context, roomID, userID int64, role string) error

	// SetRoomUserRoles は SetRoomUserRole の一括版（IN 句の UPDATE 1本）。
	// 同じ役割へ変える人がまとめて渡ってくる経路（メンバー編集）で使う。
	SetRoomUserRoles(ctx context.Context, roomID int64, userIDs []int64, role string) error
	CountRoomUsersByRole(ctx context.Context, roomID int64, role string) (int, error)
	ListRoomMembersWithRoles(ctx context.Context, roomID int64) ([]*model.RoomMember, error)
	ListRoomMembersWithRolesPage(ctx context.Context, roomID int64, q PageQuery) ([]*model.RoomMember, int, error)
	// LockRoomMemberRolesForUpdate returns the complete membership role map while
	// locking those rows. It is for business validation only and must be called
	// inside TxManager.RunInTx; display pagination must never use it.
	LockRoomMemberRolesForUpdate(ctx context.Context, roomID int64) (map[int64]string, error)
	// LockUserCommunityMembershipsForUpdate locks the target user's community
	// memberships before account deletion decides which complete rooms to lock.
	// It must be called inside TxManager.RunInTx.
	LockUserCommunityMembershipsForUpdate(ctx context.Context, userID int64) (map[int64]string, error)
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
