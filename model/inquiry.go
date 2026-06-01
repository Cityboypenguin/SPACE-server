package model

import "time"

type Inquiry struct {
	ID        string
	Name      string
	Email     string
	Subject   string
	Content   string
	CreatedAt time.Time
}
