package domain

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) (*model.User, error)
	Update(ctx context.Context, user *model.User) (*model.User, error) // 追加
	Delete(ctx context.Context, id string) (bool, error)               // 追加
	GetByID(ctx context.Context, id string) (*model.User, error)       // 追加
	GetAll(ctx context.Context) ([]*model.User, error)                 // 追加
}

type UserUsecase interface {
	CreateUser(ctx context.Context, name string, email string) (*model.User, error)
	UpdateName(ctx context.Context, id string, newName string) (*model.User, error)   // 追加
	UpdateEmail(ctx context.Context, id string, newEmail string) (*model.User, error) // 追加
	DeleteAccount(ctx context.Context, id string) (bool, error)                       // 追加
	GetUser(ctx context.Context, id string) (*model.User, error)                      // 追加
	GetUsers(ctx context.Context) ([]*model.User, error)                              // 追加
}
