package graph

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"

	"github.com/Cityboypenguin/SPACE-server/graph/model"
	domain "github.com/Cityboypenguin/SPACE-server/model"
)

func (r *mutationResolver) InviteCommunityUser(ctx context.Context, communityID string, userID string, message *string) (*model.CommunityInvitation, error) {
	community, err := decodeGraphID(ctx, "community", communityID)
	if err != nil {
		return nil, err
	}
	user, err := decodeGraphID(ctx, "user", userID)
	if err != nil {
		return nil, err
	}
	i, _, err := r.CommunityInvitations.Send(ctx, community, &user, inviteOptions(message, nil))
	if err != nil {
		return nil, err
	}
	return r.graphInvitation(ctx, i)
}

func (r *mutationResolver) InviteCommunityUsers(ctx context.Context, communityID string, userIDs []string, message *string) ([]*model.CommunityInviteResult, error) {
	community, err := decodeGraphID(ctx, "community", communityID)
	if err != nil {
		return nil, err
	}
	if len(userIDs) > communityusecase.InvitationBatchLimit {
		return nil, apperr.InvalidInput("一度に招待できるのは20人までです")
	}
	ids := make([]int64, 0, len(userIDs))
	opaque := make(map[int64]string, len(userIDs))
	for _, raw := range userIDs {
		id, err := decodeGraphID(ctx, "user", raw)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
		opaque[id] = raw
	}
	results, err := r.CommunityInvitations.SendMany(ctx, community, ids, inviteOptions(message, nil))
	if err != nil {
		return nil, err
	}
	out := make([]*model.CommunityInviteResult, 0, len(results))
	for _, result := range results {
		item := &model.CommunityInviteResult{UserID: opaque[result.RecipientID]}
		if result.Err != nil {
			msg := "招待を送れませんでした"
			var appErr *apperr.Error
			if errors.As(result.Err, &appErr) && appErr.Message != "" {
				msg = appErr.Message
			}
			item.Error = &msg
		} else if item.Invitation, err = r.graphInvitation(ctx, result.Invitation); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *mutationResolver) CreateCommunityInviteLink(ctx context.Context, communityID string, expiresInHours *int32, message *string) (*model.CommunityInviteLink, error) {
	id, err := decodeGraphID(ctx, "community", communityID)
	if err != nil {
		return nil, err
	}
	i, token, err := r.CommunityInvitations.Send(ctx, id, nil, inviteOptions(message, expiresInHours))
	if err != nil {
		return nil, err
	}
	invitation, err := r.graphInvitation(ctx, i)
	if err != nil {
		return nil, err
	}
	return &model.CommunityInviteLink{Invitation: invitation, Token: token}, nil
}

func (r *mutationResolver) AcceptCommunityInvitation(ctx context.Context, input model.CommunityInvitationReference) (string, error) {
	id, token, err := parseInvitationReference(ctx, input)
	if err != nil {
		return "", err
	}
	roomID, err := r.CommunityInvitations.Respond(ctx, id, token, true)
	if err != nil {
		return "", err
	}
	return encodeGraphID("room", roomID), nil
}

func (r *mutationResolver) DeclineCommunityInvitation(ctx context.Context, id string) (bool, error) {
	numeric, err := decodeGraphID(ctx, "communityInvitation", id)
	if err != nil {
		return false, err
	}
	_, err = r.CommunityInvitations.Respond(ctx, numeric, "", false)
	return err == nil, err
}

func (r *mutationResolver) RevokeCommunityInvitation(ctx context.Context, id string) (bool, error) {
	numeric, err := decodeGraphID(ctx, "communityInvitation", id)
	if err != nil {
		return false, err
	}
	err = r.CommunityInvitations.Revoke(ctx, numeric)
	return err == nil, err
}

func (r *mutationResolver) SendCommunityGreeting(ctx context.Context, roomID string, welcomeMessageID string, stampID string) (*model.Message, error) {
	room, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, err
	}
	welcome, err := decodeGraphID(ctx, "message", welcomeMessageID)
	if err != nil {
		return nil, err
	}
	msg, err := r.CommunityInvitations.SendGreeting(ctx, room, welcome, stampID)
	if err != nil {
		return nil, err
	}
	return toGraphMessage(msg), nil
}

func (r *mutationResolver) AdminSaveCommunityJoinStampSettings(ctx context.Context, welcomeIDs []string, greetingIDs []string) (bool, error) {
	err := r.CommunityInvitations.SaveSettings(ctx, domain.CommunityJoinStampSettings{WelcomeIDs: welcomeIDs, GreetingIDs: greetingIDs})
	return err == nil, err
}

func (r *queryResolver) ReceivedCommunityInvitations(ctx context.Context, limit *int32, offset *int32) (*model.CommunityInvitationPage, error) {
	size, start := invitationPage(limit, offset)
	rows, total, err := r.CommunityInvitations.Received(ctx, size, start)
	if err != nil {
		return nil, err
	}
	items, err := r.graphInvitations(ctx, rows)
	if err != nil {
		return nil, err
	}
	return &model.CommunityInvitationPage{Items: items, Total: total}, nil
}

func (r *queryResolver) CommunityInvitation(ctx context.Context, input model.CommunityInvitationReference) (*model.CommunityInvitation, error) {
	id, token, err := parseInvitationReference(ctx, input)
	if err != nil {
		return nil, err
	}
	i, err := r.CommunityInvitations.Preview(ctx, id, token)
	if err != nil {
		return nil, err
	}
	return r.graphInvitation(ctx, i)
}

func (r *queryResolver) SentCommunityInvitations(ctx context.Context, roomID string, history *bool, limit *int32, offset *int32) ([]*model.CommunityInvitation, error) {
	id, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, err
	}
	size, start := invitationPage(limit, offset)
	rows, err := r.CommunityInvitations.Sent(ctx, id, history != nil && *history, size, start)
	if err != nil {
		return nil, err
	}
	return r.graphInvitations(ctx, rows)
}

func (r *queryResolver) CommunityInviteCandidates(ctx context.Context, communityID string, keyword string, limit *int32, offset *int32) (*model.CommunityInviteCandidatePage, error) {
	id, err := decodeGraphID(ctx, "community", communityID)
	if err != nil {
		return nil, err
	}
	users, total, err := r.SearchUsersUseCase.Execute(ctx, keyword, resolvePageQuery(ctx, limit, offset, defaultPageSize))
	if err != nil {
		return nil, err
	}
	candidates, err := r.CommunityInvitations.Candidates(ctx, id, users)
	if err != nil {
		return nil, err
	}
	items := make([]*model.CommunityInviteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, &model.CommunityInviteCandidate{User: toGraphUser(candidate.User), Status: candidate.State})
	}
	return &model.CommunityInviteCandidatePage{Items: items, Total: int32(total)}, nil
}

func (r *queryResolver) CommunityGreeting(ctx context.Context, roomID string) (*model.CommunityGreeting, error) {
	id, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, err
	}
	target, candidates, err := r.CommunityInvitations.Greeting(ctx, id)
	if err != nil || target == nil {
		return nil, err
	}
	stamps := make([]*model.ChatStamp, 0, len(candidates))
	for _, stamp := range candidates {
		stamps = append(stamps, r.graphStamp(stamp))
	}
	return &model.CommunityGreeting{WelcomeMessageID: encodeGraphID("message", target.MessageID), JoinedUserName: target.UserName, Stamps: stamps}, nil
}

func (r *queryResolver) AdminCommunityJoinStampSettings(ctx context.Context) (*model.CommunityJoinStampSettings, error) {
	settings, err := r.CommunityInvitations.AdminSettings(ctx)
	if err != nil {
		return nil, err
	}
	return &model.CommunityJoinStampSettings{WelcomeIDs: settings.WelcomeIDs, GreetingIDs: settings.GreetingIDs}, nil
}
