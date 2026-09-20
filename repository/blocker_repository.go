package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type BlockerRepository interface {
	CreateBlocker(ctx context.Context, blocker *model.Blocker) (int64, error)
	DeleteBlocker(ctx context.Context, userID int64, blockedID int64) (bool, error)
	ListBlockers(ctx context.Context, userID int64, q PageQuery) ([]*model.Blocker, int, error)
	// この2つは以前は窓を取らず全件返していた（ListBlockers は最初から
	// PageQuery を取っている）。total は返さない＝呼び出し元が [User!]! を返すので
	// 数える先が無い。
	SearchBlockers(ctx context.Context, userID int64, keyword string, q PageQuery) ([]*model.Blocker, error)
	GetBlockersByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Blocker, error)
	ExistsBlockRelation(ctx context.Context, userA int64, userB int64) (bool, error)
	GetBlockedAndBlockerIDs(ctx context.Context, userID int64) ([]int64, error)
}
