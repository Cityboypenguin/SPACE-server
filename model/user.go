package model

import "time"

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

func NewUser(param CreateUserParam) *User {
	return &User{
		Name:      param.Name,
		CreatedAt: param.CreatedAt,
		UpdatedAt: param.UpdatedAt,
	}
}
