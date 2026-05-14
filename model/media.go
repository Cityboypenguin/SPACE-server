package model

import "time"

type Media struct {
	ID             int64
	UploaderUserID int64
	StorageKey     string
	ContentType    string
	CreatedAt      time.Time
}
