package model

import (
	"time"
)

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateUserParam struct {
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (u *User) Create(params CreateUserParam) error {
	u.Name = params.Name
	u.CreatedAt = params.CreatedAt
	u.UpdatedAt = params.UpdatedAt

	return nil
}
