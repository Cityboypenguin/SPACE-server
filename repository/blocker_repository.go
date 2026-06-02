package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type BlockerRepository interface {
	CreateBlocker(ctx context.Context, blocker *model.Blocker) (int64, error)
	DeleteBlocker(ctx context.Context, userID int64, blockedID int64) (bool, error)
	ListBlockers(ctx context.Context) ([]*model.Blocker, error)
	SearchBlockers(ctx context.Context, userID int64, keyword string) ([]*model.Blocker, error)
	GetBlockersByUserID(ctx context.Context, userID int64) ([]*model.Blocker, error)
	ExistsBlockRelation(ctx context.Context, userA int64, userB int64) (bool, error)
	GetBlockedAndBlockerIDs(ctx context.Context, userID int64) ([]int64, error)
}
