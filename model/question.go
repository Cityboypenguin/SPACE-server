package model

import "time"

type Question struct {
	ID           int64
	RoomID       int64
	AskerUserID  int64
	AuthorRole   string
	Body         string
	IsAnswered   bool
	BestAnswerID *int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
