package model

import "time"

type Announcement struct {
	ID        int64
	Title     string
	Body      string
	AdminID   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
