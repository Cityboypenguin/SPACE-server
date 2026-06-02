package model

import "time"

type Blocker struct {
	ID            int64
	UserID        int64
	BlockedUserID int64
	CreatedAt     time.Time
}

type CreateBlockerParam struct {
	UserID        int64
	BlockedUserID int64
}

func CreateBlocker(param CreateBlockerParam) *Blocker {
	return &Blocker{
		UserID:        param.UserID,
		BlockedUserID: param.BlockedUserID,
		CreatedAt:     time.Now(),
	}
}
