package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

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
}
