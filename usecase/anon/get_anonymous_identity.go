package anon

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// GetAnonymousIdentityUseCase は匿名ID（匿名NNN）を採番せずに引くだけの口。
//
// 表示（GraphQL の query / field resolver）はこちらを使う。以前は表示時に
// GetOrCreate を呼んでいたため、読むだけのクエリが DB に行を作る副作用を持ち、
// しかも番号が「初投稿順」ではなく「初めて誰かの画面に出た順」になりえた。
// 採番は書き込み経路（usecase/chat・質問・回答・投票の作成時）だけが行う。
type GetAnonymousIdentityUseCase interface {
	// Execute returns nil (with no error) when userID has never posted in roomID.
	// 呼び出し側はそのとき実名へフォールバックしてはいけない（匿名性が壊れる）。
	Execute(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error)
}

var _ GetAnonymousIdentityUseCase = &GetAnonymousIdentityInteractor{}

type GetAnonymousIdentityInteractor struct {
	identityRepo repository.RoomAnonymousIdentityRepository
}

func NewGetAnonymousIdentityUseCase(identityRepo repository.RoomAnonymousIdentityRepository) GetAnonymousIdentityUseCase {
	return &GetAnonymousIdentityInteractor{identityRepo: identityRepo}
}

// Execute is called from the GraphQL layer only after the caller has already been
// authenticated (it is not itself exposed as an API field), so it does not repeat
// an authz.RequireAuth check.
func (uc *GetAnonymousIdentityInteractor) Execute(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	return uc.identityRepo.Get(ctx, roomID, userID)
}
