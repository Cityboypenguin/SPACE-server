package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type UserRepository interface {
	SaveUser(ctx context.Context, user *model.User) error
}
