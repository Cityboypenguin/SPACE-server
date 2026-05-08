package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	UserStatusActive = "active"
	UserStatusFrozen = "frozen"
)

type User struct {
	ID             int64
	AccountID      string
	Name           string
	Email          string
	HashedPassword string
	Role           string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateUserParam struct {
	AccountID string
	Name      string
	Email     string
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdateUserParam struct {
	AccountID *string
	Name      *string
	Email     *string
	Password  *string
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
	if param.AccountID != nil {
		u.AccountID = *param.AccountID
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
