package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type UserRepository interface {
	SaveUser(ctx context.Context, u *model.User) error
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	DeleteUser(ctx context.Context, id int64) (bool, error)
	ListUsers(ctx context.Context) ([]*model.User, error)
	UpdateUser(ctx context.Context, u *model.User) error
	SearchUsersByName(ctx context.Context, name string) ([]*model.User, error)
}
