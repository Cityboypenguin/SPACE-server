package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID             int64     `json:"id"`
	AccountID      string    `json:"account_id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	HashedPassword string    `json:"hashed_password"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateUserParam struct {
	AccountID string    `json:"account_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateUserParam struct {
	UserID   *string `json:"user_id,omitempty"`
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
}

func hashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

func (u *User) CreateUser(param CreateUserParam) error {
	hashedPassword, err := hashPassword(param.Password)
	if err != nil {
		return err
	}

	u.AccountID = param.AccountID
	u.Name = param.Name
	u.Email = param.Email
	u.HashedPassword = hashedPassword
	u.CreatedAt = param.CreatedAt
	u.UpdatedAt = param.UpdatedAt

	return nil
}

func (u *User) UpdateUser(param UpdateUserParam) error {
	if param.UserID != nil {
		u.UserID = *param.UserID
	}
	if param.Name != nil {
		u.Name = *param.Name
	}
	if param.Email != nil {
		u.Email = *param.Email
	}
	if param.Password != nil {
		hashedPassword, err := hashPassword(*param.Password)
		if err != nil {
			return err
		}
		u.HashedPassword = hashedPassword
	}
	u.UpdatedAt = time.Now()
	return nil
}
