package anon

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetAnonymousIdentitiesUseCase は (roomID, userID) の集合の匿名IDをまとめて引く
// （DataLoader 用）。GetAnonymousIdentityUseCase と同じく採番はしない。
//
// 一覧の1画面に同じ投稿者が何度も出るので、キャッシュが効くだけでも効果が大きい。
// さらに Message は userID と user の2フィールドで同じ解決を通るため、
// 1メッセージあたり2回引いていた。
type GetAnonymousIdentitiesUseCase interface {
	// Execute returns only the keys that have a row. 行が無い key は map に入らない。
	// 呼び出し側はそのとき実名へフォールバックしてはいけない（匿名性が壊れる）。
	Execute(ctx context.Context, keys []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error)
}

var _ GetAnonymousIdentitiesUseCase = &GetAnonymousIdentitiesInteractor{}

type GetAnonymousIdentitiesInteractor struct {
	identityRepo repository.RoomAnonymousIdentityRepository
}

func NewGetAnonymousIdentitiesUseCase(identityRepo repository.RoomAnonymousIdentityRepository) GetAnonymousIdentitiesUseCase {
	return &GetAnonymousIdentitiesInteractor{identityRepo: identityRepo}
}

// Execute is called from the GraphQL layer only after the caller has already been
// authenticated, so—like GetAnonymousIdentityUseCase—it does not repeat an
// authz.RequireAuth check.
func (uc *GetAnonymousIdentitiesInteractor) Execute(ctx context.Context, keys []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error) {
	return uc.identityRepo.GetByRoomUserKeys(ctx, keys)
}
