package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type UserRepository interface {
	SaveUser(ctx context.Context, user *model.User) error
	GetUser(ctx context.Context, id int64) (*model.User, error)
	GetUsers(ctx context.Context) ([]*model.User, error)
}
