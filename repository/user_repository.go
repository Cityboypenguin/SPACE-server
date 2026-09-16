package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type UserRepository interface {
	SaveUser(ctx context.Context, u *model.User) error
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	GetUsersByIDs(ctx context.Context, ids []int64) ([]*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByAccountID(ctx context.Context, accountID string) (*model.User, error)
	DeleteUser(ctx context.Context, id int64) (bool, error)
	ListUsers(ctx context.Context, limit, offset int) ([]*model.User, int, error)
	UpdateUser(ctx context.Context, u *model.User) error
	SearchUsersByKeyword(ctx context.Context, keyword string, limit, offset int) ([]*model.User, int, error)
	// GetUsersByAccountIDs は accountID からユーザーをまとめて引く（メンション解決用）。
	// 照合は DB の照合順序に従うため、大文字小文字は区別しない。
	GetUsersByAccountIDs(ctx context.Context, accountIDs []string) ([]*model.User, error)
	// SuggestUsersByPrefix は accountID が prefix に前方一致するユーザーを返す（メンションのサジェスト用）。
	SuggestUsersByPrefix(ctx context.Context, prefix string, limit int) ([]*model.User, error)
	UpdateLastActiveAt(ctx context.Context, userID int64, now int64) error
	LogActivityDate(ctx context.Context, userID int64, jstDate string) error
}
