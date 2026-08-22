package model

import "time"

type Answer struct {
	ID           int64
	QuestionID   int64
	AuthorUserID int64
	AuthorRole   string
	Body         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
