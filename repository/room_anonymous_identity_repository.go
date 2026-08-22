package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomAnonymousIdentityRepository interface {
	// GetOrCreate returns the fixed anonymous identity for (roomID, userID), allocating
	// the next sequence number ("匿名001", "匿名002", ...) for that room on first use.
	// The same user always gets the same identity within a given room (F-05: 匿名IDは
	// 授業ごとに固定).
	GetOrCreate(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error)
}
