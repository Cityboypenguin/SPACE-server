package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// AdminListCommunitiesUseCase は管理画面のコミュニティ一覧。
// 検索・並び替え・ページングは DB 側で行い、オーナーは一覧ぶんを1クエリで引く。
type AdminListCommunitiesUseCase interface {
	Execute(ctx context.Context, f repository.AdminCommunityFilter, q repository.PageQuery) ([]*model.Community, int, error)
	// ListOwners は roomID ごとのオーナー（いなければ key ごと欠ける）を返す。
	ListOwners(ctx context.Context, roomIDs []int64) (map[int64]*model.User, error)
}

var _ AdminListCommunitiesUseCase = &AdminListCommunitiesInteractor{}

type AdminListCommunitiesInteractor struct {
	communityRepo repository.CommunityRepository
	roomUserRepo  repository.RoomMembershipReader
}

func NewAdminListCommunitiesUseCase(communityRepo repository.CommunityRepository, roomUserRepo repository.RoomMembershipReader) AdminListCommunitiesUseCase {
	return &AdminListCommunitiesInteractor{communityRepo: communityRepo, roomUserRepo: roomUserRepo}
}

func (uc *AdminListCommunitiesInteractor) Execute(ctx context.Context, f repository.AdminCommunityFilter, q repository.PageQuery) ([]*model.Community, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.communityRepo.AdminListCommunities(ctx, f, q)
}

func (uc *AdminListCommunitiesInteractor) ListOwners(ctx context.Context, roomIDs []int64) (map[int64]*model.User, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.roomUserRepo.ListOwnersByRoomIDs(ctx, roomIDs)
}
