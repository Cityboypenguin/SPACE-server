package graph

import (
	"context"
	"errors"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// checkCanLeaveCommunity returns an error if the leaving user is the sole owner
// while other members remain, preventing an ownerless community.
func (r *mutationResolver) checkCanLeaveCommunity(ctx context.Context, roomID, leavingUserID int64) error {
	members, err := r.ListRoomMembersWithRolesUseCase.Execute(ctx, roomID)
	if err != nil {
		return err
	}

	leavingIsOwner := false
	ownerCount := 0
	hasOtherMembers := false
	for _, m := range members {
		if m.Role == model.RoomUserRoleOwner {
			ownerCount++
			if m.User.ID == leavingUserID {
				leavingIsOwner = true
			}
		}
		if m.User.ID != leavingUserID {
			hasOtherMembers = true
		}
	}

	if leavingIsOwner && ownerCount == 1 && hasOtherMembers {
		return errors.New("cannot leave: you are the last owner; transfer ownership to another member first")
	}
	return nil
}

func (r *mutationResolver) deleteCommunityIfEmpty(ctx context.Context, roomID int64) error {
	memberIDs, err := r.GetUserIDsByRoomIDUseCase.Execute(ctx, roomID)
	if err != nil {
		return err
	}
	if len(memberIDs) == 0 {
		if _, err := r.DeleteRoomUseCase.Execute(ctx, roomID); err != nil {
			logger.Log.Error().Err(err).Int64("room_id", roomID).Msg("failed to delete empty community room")
			return err
		}
	}
	return nil
}

func (r *mutationResolver) countCommunityOwners(ctx context.Context, roomID int64) (int, error) {
	members, err := r.ListRoomMembersWithRolesUseCase.Execute(ctx, roomID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, m := range members {
		if m.Role == model.RoomUserRoleOwner {
			count++
		}
	}
	return count, nil
}

// communityMembership はコミュニティ表示に足す2つの集計（人数・自分の所属）を、
// 一覧ぶんまとめて1回で用意した結果。
//
// 以前は表示するコミュニティ1件ごとに GetUserIDsByRoomID でメンバーIDを全部取り、
// その len を人数に、containsInt64 を所属フラグにしていた。一覧20件なら20クエリで、
// しかも人数とフラグしか使わないのに全メンバーの行を持ち帰っていた。
// どちらも roomID の集合に対する1クエリで出せる。
//
// 「引いていない」を空の map で表すと、引いた上で0人だったのか、要求されて
// いないので引いていないのかを区別できない。notificationHydration と同じく
// loaded で示す。
type communityMembership struct {
	counts       map[int64]int
	countsLoaded bool
	joined       map[int64]bool
	joinedLoaded bool
}

// loadCommunityMembership は memberCount / isMember のうち要求されたものだけを、
// それぞれ1クエリで用意する。
//
// viewerUserID が nil のとき（管理者の全件一覧）は所属フラグを引かない。
// 「誰の所属か」が決まらないので、以前から false 固定だった経路。
//
// 判定は共通の fieldRequested。ここでは呼び出し側が渡した want* をそのまま使う
// （このヘルパーは一覧・単体の両方から呼ばれ、選択パスの深さが違うため）。
//
// DataLoader を使わないのは、ここが親リゾルバだから。親が roomID の集合を
// 既に持っているので、LoadAll で積み直しても結局同じ1クエリになる。通知の
// actor/post（hydrateNotifications）と同じ形に揃えておく。
func (r *Resolver) loadCommunityMembership(ctx context.Context, roomIDs []int64, viewerUserID *int64, wantCount, wantMember bool) (communityMembership, error) {
	m := communityMembership{}

	if wantCount {
		counts, err := r.CountUsersByRoomIDsUseCase.Execute(ctx, roomIDs)
		if err != nil {
			return communityMembership{}, err
		}
		m.counts = counts
		m.countsLoaded = true
	}

	if wantMember && viewerUserID != nil {
		joined, err := r.ListJoinedRoomIDsUseCase.Execute(ctx, *viewerUserID, roomIDs)
		if err != nil {
			return communityMembership{}, err
		}
		m.joined = joined
		m.joinedLoaded = true
	}

	return m, nil
}

// toGraphCommunityWith は Community を GraphQL 用に変換し、用意済みの集計を写す。
// 引いていない集計はゼロ値のまま（要求されていないので応答には出ない）。
func (r *Resolver) toGraphCommunityWith(c *model.Community, m communityMembership) *gqlmodel.Community {
	gqlCommunity := toGraphCommunity(c, r.communityAvatarURL(c))
	if gqlCommunity == nil {
		return nil
	}
	if m.countsLoaded {
		gqlCommunity.MemberCount = int32(m.counts[c.RoomID])
	}
	if m.joinedLoaded {
		gqlCommunity.IsMember = m.joined[c.RoomID]
	}
	return gqlCommunity
}

// communityRoomIDs は一覧から roomID を取り出す（集計の IN に渡すため）。
// nil 要素は飛ばす。以前は1件ずつ変換していて nil でも落ちなかったので、
// 集計を先に引く形にしたことで新しく落ちるようにはしない。
func communityRoomIDs(communities []*model.Community) []int64 {
	ids := make([]int64, 0, len(communities))
	for _, c := range communities {
		if c == nil {
			continue
		}
		ids = append(ids, c.RoomID)
	}
	return ids
}
