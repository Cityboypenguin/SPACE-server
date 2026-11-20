package community

import (
	"context"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/usecase/chat"
)

// Execute is the normal self-join entry point. Invitations use the same join path.
func (s *InvitationService) Execute(ctx context.Context, roomID int64) (bool, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return false, err
	}
	room, roles, msg, err := s.withRoomMembership(ctx, roomID, func(ctx context.Context, room *model.Room, roles map[int64]string) (*model.Message, error) {
		return s.join(ctx, room, roles, actor)
	})
	if err != nil {
		return false, err
	}
	s.publish(ctx, room, roles, msg)
	return true, nil
}

func (s *InvitationService) withRoomMembership(ctx context.Context, roomID int64, fn func(context.Context, *model.Room, map[int64]string) (*model.Message, error)) (room *model.Room, roles map[int64]string, msg *model.Message, err error) {
	err = s.Tx.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		roles, err = s.Members.LockRoomMemberRolesForUpdate(ctx, roomID)
		if err != nil {
			return err
		}
		room, err = s.Rooms.GetRoomByID(ctx, roomID)
		if err != nil {
			return err
		}
		if room == nil || room.Type != model.RoomTypeCommunity || room.ParentRoomID != nil {
			return apperr.NotFound("コミュニティが見つかりません")
		}
		msg, err = fn(ctx, room, roles)
		return err
	})
	return room, roles, msg, err
}

func (s *InvitationService) join(ctx context.Context, room *model.Room, roles map[int64]string, actor int64) (*model.Message, error) {
	if err := s.Store.AcceptReceived(ctx, room.ID, actor, time.Now()); err != nil {
		return nil, err
	}
	if roles[actor] != "" {
		return nil, nil
	}
	user, err := s.Users.GetUserByID(ctx, actor)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperr.NotFound("ユーザーが見つかりません")
	}
	if err := s.Members.AddUserToRoom(ctx, room.ID, actor); err != nil {
		return nil, err
	}
	roles[actor] = model.RoomUserRoleMember

	settings, err := s.Store.JoinStampSettings(ctx)
	if err != nil {
		return nil, err
	}
	candidates, err := s.activeStamps(ctx, settings.WelcomeIDs)
	if err != nil {
		return nil, err
	}
	var stampID *string
	if len(candidates) > 0 {
		stampID = &candidates[rand.IntN(len(candidates))].ID
	}
	msg, err := s.Messages.SaveCommunityWelcome(ctx, room.ID, user.ID, stampID)
	if err != nil {
		return nil, err
	}
	return msg, s.Store.SetWelcomeMessage(ctx, room.ID, actor, msg.ID)
}

func (s *InvitationService) publish(ctx context.Context, room *model.Room, roles map[int64]string, msg *model.Message) {
	if msg == nil {
		return
	}
	members := make([]int64, 0, len(roles))
	for id := range roles {
		members = append(members, id)
	}
	s.Events.MessageSent(ctx, chat.MessageSentEvent{Room: room, Message: msg, ActorID: msg.UserID, MemberIDs: members})
}

func (s *InvitationService) activeStamps(ctx context.Context, ids []string) ([]*model.ChatStamp, error) {
	stamps := make([]*model.ChatStamp, 0, len(ids))
	if len(ids) == 0 {
		return stamps, nil
	}
	catalog, err := s.Stamps.List(ctx)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]*model.ChatStamp, len(catalog))
	for _, stamp := range catalog {
		if stamp.Status == "active" && stamp.AllowMessage {
			allowed[stamp.ID] = stamp
		}
	}
	for _, id := range ids {
		if stamp := allowed[id]; stamp != nil {
			stamps = append(stamps, stamp)
		}
	}
	return stamps, nil
}

func (s *InvitationService) AdminSettings(ctx context.Context) (model.CommunityJoinStampSettings, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return model.CommunityJoinStampSettings{}, err
	}
	return s.Store.JoinStampSettings(ctx)
}

func (s *InvitationService) SaveSettings(ctx context.Context, settings model.CommunityJoinStampSettings) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	for _, ids := range [][]string{settings.WelcomeIDs, settings.GreetingIDs} {
		if len(ids) > 50 {
			return apperr.InvalidInput("候補はそれぞれ50件までです")
		}
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if seen[id] {
				return apperr.InvalidInput("同じスタンプは複数回登録できません")
			}
			seen[id] = true
		}
		available, err := s.activeStamps(ctx, ids)
		if err != nil {
			return err
		}
		if len(available) != len(ids) {
			return apperr.InvalidInput("メッセージに使用できる公開中のスタンプを選択してください")
		}
	}
	return s.Store.SaveJoinStampSettings(ctx, settings)
}

func (s *InvitationService) Greeting(ctx context.Context, roomID int64) (*int64, []*model.ChatStamp, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, nil, err
	}
	welcomeID, err := s.Store.WelcomeMessage(ctx, roomID, actor)
	if err != nil || welcomeID == nil {
		return nil, nil, err
	}
	settings, err := s.Store.JoinStampSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	options, err := s.activeStamps(ctx, settings.GreetingIDs)
	if err != nil || len(options) == 0 {
		return nil, nil, err
	}
	return welcomeID, options, nil
}

func (s *InvitationService) SendGreeting(ctx context.Context, roomID, welcomeID int64, stampID string) (*model.Message, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	room, roles, msg, err := s.withRoomMembership(ctx, roomID, func(ctx context.Context, room *model.Room, roles map[int64]string) (*model.Message, error) {
		if roles[actor] == "" {
			return nil, apperr.Forbidden("コミュニティのメンバーのみ送信できます")
		}
		expectedID, options, err := s.Greeting(ctx, room.ID)
		if err != nil {
			return nil, err
		}
		if expectedID == nil || *expectedID != welcomeID {
			return nil, apperr.Conflict("この参加案内への挨拶は終了しました")
		}
		if !slices.ContainsFunc(options, func(stamp *model.ChatStamp) bool { return stamp.ID == stampID }) {
			return nil, apperr.InvalidInput("現在使える「よろしく」スタンプを選択してください")
		}
		msg, err := s.Messages.SaveCommunityGreeting(ctx, room.ID, actor, stampID)
		if err != nil {
			return nil, err
		}
		return msg, s.Store.MarkGreetingSent(ctx, room.ID, actor)
	})
	if err != nil {
		return nil, err
	}
	s.publish(ctx, room, roles, msg)
	return msg, nil
}
