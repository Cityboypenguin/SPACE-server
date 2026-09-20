package model

import "time"

type RoomAnonymousIdentity struct {
	ID        int64
	RoomID    int64
	UserID    int64
	Label     string
	CreatedAt time.Time
}
