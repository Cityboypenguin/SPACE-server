package graph

import (
	"context"
	"time"

	gql "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
)

func parseInvitationReference(ctx context.Context, ref gql.CommunityInvitationReference) (int64, string, error) {
	if (ref.ID == nil) == (ref.Token == nil) {
		return 0, "", apperr.InvalidInput("招待の指定が不正です")
	}
	if ref.Token != nil {
		if len(*ref.Token) != 43 {
			return 0, "", apperr.InvalidInput("招待リンクが不正です")
		}
		return 0, *ref.Token, nil
	}
	id, err := decodeGraphID(ctx, "communityInvitation", *ref.ID)
	return id, "", err
}

func invitationPage(limit, offset *int32) (int32, int32) {
	size, start := int32(20), int32(0)
	if limit != nil {
		size = min(max(*limit, 0), 50)
	}
	if offset != nil {
		start = max(*offset, 0)
	}
	return size, start
}

func (r *Resolver) graphInvitation(ctx context.Context, i *model.CommunityInvitation) (*gql.CommunityInvitation, error) {
	items, err := r.graphInvitations(ctx, []*model.CommunityInvitation{i})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, apperr.NotFound("この招待は利用できません")
	}
	return items[0], nil
}

func (r *Resolver) graphInvitations(ctx context.Context, rows []*model.CommunityInvitation) ([]*gql.CommunityInvitation, error) {
	result := make([]*gql.CommunityInvitation, 0, len(rows))
	if len(rows) == 0 {
		return result, nil
	}
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	service := r.CommunityInvitations
	userIDs := make([]int64, 0, len(rows)*2)
	roomIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.SenderID)
		if row.RecipientID != nil {
			userIDs = append(userIDs, *row.RecipientID)
		}
		roomIDs = append(roomIDs, row.RoomID)
	}
	users, err := service.Users.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]*string, len(users))
	for _, user := range users {
		names[user.ID] = &user.Name
	}
	memberships, err := service.Members.ListJoinedRoomIDs(ctx, claims.ID, roomIDs)
	if err != nil {
		return nil, err
	}
	communities := make(map[int64]*model.Community)
	for _, row := range rows {
		c, seen := communities[row.RoomID]
		if !seen {
			communityID, err := service.Communities.GetCommunityIDByRoomID(ctx, row.RoomID)
			if err != nil {
				return nil, err
			}
			if communityID != 0 {
				c, err = service.Communities.GetCommunityByID(ctx, communityID)
				if err != nil {
					return nil, err
				}
			}
			communities[row.RoomID] = c
		}
		if c == nil {
			continue
		}
		invite := &gql.CommunityInvitation{
			ID: encodeGraphID("communityInvitation", row.ID), RoomID: encodeGraphID("room", row.RoomID),
			CommunityName: c.Name, CommunityDescription: c.Description, SenderName: names[row.SenderID],
			IsLink: row.RecipientID == nil, IsMember: memberships[row.RoomID], Status: row.Status(time.Now()), ExpiresAt: row.ExpiresAt.Format(timeFormat),
		}
		if row.RecipientID != nil {
			invite.RecipientName = names[*row.RecipientID]
		}
		result = append(result, invite)
	}
	return result, nil
}
