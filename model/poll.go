package model

import "time"

type Poll struct {
	ID                  int64
	RoomID              int64
	AuthorUserID        int64
	AuthorRole          string
	Question            string
	AllowMultipleChoice bool
	Deadline            *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type PollOption struct {
	ID           int64
	PollID       int64
	Label        string
	DisplayOrder int
}

type PollVote struct {
	ID           int64
	PollOptionID int64
	UserID       int64
	CreatedAt    time.Time
}
