package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type RoomAnonymousIdentityRepository interface {
	// GetOrCreate returns the fixed anonymous identity for (roomID, userID), allocating
	// the next sequence number ("匿名001", "匿名002", ...) for that room on first use.
	// The same user always gets the same identity within a given room (F-05: 匿名IDは
	// 授業ごとに固定). Numbers are only consumed by users who post: a row created by
	// merely reading (UpsertLastReadAt) has no label until its first post.
	GetOrCreate(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error)
	// UpsertLastReadAt records how far userID has read in a course room, creating the
	// row (without a label) if the user has not posted there yet.
	UpsertLastReadAt(ctx context.Context, roomID, userID, readAt int64) error
	// GetLastReadAt returns nil when userID has never read the room.
	GetLastReadAt(ctx context.Context, roomID, userID int64) (*int64, error)
}
