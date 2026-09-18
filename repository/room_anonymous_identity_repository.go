package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// RoomUserKey は匿名IDを引くときの複合キー (room_id, user_id)。
// 匿名IDは「ルームごとに固定」なので、この2つが揃って初めて1行に定まる。
// comparable なので DataLoader のキーにそのまま使える。
type RoomUserKey struct {
	RoomID int64
	UserID int64
}

// RoomAnonymousIdentityRepository は授業内チャットの匿名表示名（匿名NNN）だけを扱う。
// 既読位置は CourseRoomReadRepository が持つ（course_room_reads という別表）。
type RoomAnonymousIdentityRepository interface {
	// GetOrCreate returns the fixed anonymous identity for (roomID, userID), allocating
	// the next sequence number ("匿名001", "匿名002", ...) for that room on first use.
	// The same user always gets the same identity within a given room (F-05: 匿名IDは
	// 授業ごとに固定). 行ができるのは投稿したときだけなので、番号は投稿した順に埋まる。
	GetOrCreate(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error)
	// Get returns the identity without allocating one, or nil if userID has never
	// posted in the room. 表示側（読むだけの画面）が番号を消費しないための口。
	Get(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error)
	// GetByRoomUserKeys は複数の (roomID, userID) の匿名IDを1クエリでまとめて引く
	// （DataLoader 用）。Get と同じく採番はしない。
	//
	// 返す map には行が見つかった key だけを入れる。見つからない key は落とす
	// （Get が「無ければ nil, nil」を返すのと同じ扱い）。呼び出し側は
	// 「引けなかったら実名」ではなく「引けなかったら番号なしの匿名」へ倒すこと。
	GetByRoomUserKeys(ctx context.Context, keys []RoomUserKey) (map[RoomUserKey]*model.RoomAnonymousIdentity, error)
}
