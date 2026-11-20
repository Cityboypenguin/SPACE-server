package community

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/chat"
	"github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

const invitationLifetime = 7 * 24 * time.Hour
const invitationLimitPerHour = 20

type invitationStore interface {
	Create(context.Context, *model.CommunityInvitation, []byte) error
	Get(context.Context, int64, bool) (*model.CommunityInvitation, error)
	GetByToken(context.Context, []byte) (*model.CommunityInvitation, error)
	Pending(context.Context, int64, int64, time.Time) (*model.CommunityInvitation, error)
	ListReceived(context.Context, int64, time.Time, int32, int32) ([]*model.CommunityInvitation, int32, error)
	ListSent(context.Context, int64, int64, bool, time.Time, int32, int32) ([]*model.CommunityInvitation, error)
	RecentCount(context.Context, int64, int64, time.Time) (int, error)
	RecentlyDeclined(context.Context, int64, int64, time.Time) (bool, error)
	SetState(context.Context, int64, string) error
	AcceptReceived(context.Context, int64, int64, time.Time) error
	JoinStampSettings(context.Context) (model.CommunityJoinStampSettings, error)
	SaveJoinStampSettings(context.Context, model.CommunityJoinStampSettings) error
	WelcomeTarget(context.Context, int64, int64) (*model.CommunityWelcomeTarget, error)
	SetWelcomeMessage(context.Context, int64, int64, int64) error
	MarkWelcomeSent(context.Context, int64, int64) error
}

type InvitationService struct {
	Store         invitationStore
	Rooms         repository.RoomRepository
	Members       repository.RoomUserRepository
	Communities   repository.CommunityRepository
	Users         repository.UserReader
	Blocks        repository.BlockerRepository
	Tx            repository.TxManager
	Notifications notification.NotificationPublisher
	Stamps        repository.ChatStampRepository
	Messages      chat.MessageWriters
	Events        chat.EventPublisher
}

func invitationUnavailable() error {
	return apperr.NotFound("この招待は利用できません")
}

func (s *InvitationService) Send(ctx context.Context, communityID int64, recipientID *int64) (*model.CommunityInvitation, string, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, "", err
	}
	c, err := s.Communities.GetCommunityByID(ctx, communityID)
	if err != nil {
		return nil, "", err
	}
	if c == nil {
		return nil, "", apperr.NotFound("コミュニティが見つかりません")
	}
	now := time.Now().Truncate(time.Second)
	i := &model.CommunityInvitation{RoomID: c.RoomID, SenderID: actor, RecipientID: recipientID,
		State: model.CommunityInvitationPending, CreatedAt: now, ExpiresAt: now.Add(invitationLifetime)}
	var token string
	var digest []byte
	if recipientID == nil {
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, "", err
		}
		token = base64.RawURLEncoding.EncodeToString(raw[:])
		hash := sha256.Sum256(raw[:])
		digest = hash[:]
	}
	created := false
	err = s.Tx.RunInTx(ctx, func(ctx context.Context) error {
		roles, err := s.Members.LockRoomMemberRolesForUpdate(ctx, c.RoomID)
		if err != nil {
			return err
		}
		if roles[actor] == "" {
			return apperr.Forbidden("コミュニティのメンバーのみ招待できます")
		}
		if recipientID != nil {
			if *recipientID == actor || roles[*recipientID] != "" {
				return apperr.InvalidInput("このユーザーを招待できません")
			}
			target, err := s.Users.GetUserByID(ctx, *recipientID)
			if err != nil {
				return err
			}
			blocked, err := s.Blocks.ExistsBlockRelation(ctx, actor, *recipientID)
			if err != nil {
				return err
			}
			if target == nil || blocked {
				return apperr.InvalidInput("このユーザーを招待できません")
			}
			existing, err := s.Store.Pending(ctx, c.RoomID, *recipientID, now)
			if err != nil {
				return err
			}
			if existing != nil {
				if existing.SenderID != actor {
					return apperr.Conflict("このユーザーを招待できません")
				}
				i = existing
				return nil
			}
			declined, err := s.Store.RecentlyDeclined(ctx, c.RoomID, *recipientID, now)
			if err != nil {
				return err
			}
			if declined {
				return apperr.Conflict("このユーザーを招待できません")
			}
		}
		count, err := s.Store.RecentCount(ctx, c.RoomID, actor, now.Add(-time.Hour))
		if err != nil {
			return err
		}
		if count >= invitationLimitPerHour {
			return apperr.Conflict("招待の送信上限に達しました。時間をおいて再試行してください")
		}
		if err := s.Store.Create(ctx, i, digest); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if created && recipientID != nil {
		target := notification.TargetCommunityInvitation
		err := s.Notifications.Publish(ctx, notification.PublishParams{
			UserID: *recipientID, ActorID: &actor, TargetType: &target, TargetID: &i.ID,
			Type: notification.TypeCommunityInvitation, Message: c.Name + "に招待されました",
		})
		if err != nil {
			logger.Log.Error().Err(err).Int64("invitation_id", i.ID).Msg("community invitation notification failed")
		}
	}
	return i, token, nil
}

func (s *InvitationService) Received(ctx context.Context, limit, offset int32) ([]*model.CommunityInvitation, int32, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.Store.ListReceived(ctx, actor, time.Now(), limit, offset)
}

func (s *InvitationService) Sent(ctx context.Context, roomID int64, limit, offset int32) ([]*model.CommunityInvitation, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := s.Members.GetRoomUserRole(ctx, roomID, actor)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, apperr.Forbidden("コミュニティのメンバーのみ確認できます")
	}
	return s.Store.ListSent(ctx, roomID, actor, role == model.RoomUserRoleOwner, time.Now(), limit, offset)
}

func (s *InvitationService) load(ctx context.Context, actor, id int64, token string) (*model.CommunityInvitation, error) {
	if token != "" {
		raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
		if err != nil || len(token) != 43 || len(raw) != 32 {
			return nil, invitationUnavailable()
		}
		digest := sha256.Sum256(raw)
		i, err := s.Store.GetByToken(ctx, digest[:])
		if err != nil {
			return nil, err
		}
		if i == nil || i.RecipientID != nil {
			return nil, invitationUnavailable()
		}
		return i, nil
	}
	i, err := s.Store.Get(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if i == nil || i.RecipientID == nil || *i.RecipientID != actor {
		return nil, invitationUnavailable()
	}
	return i, nil
}

func (s *InvitationService) available(ctx context.Context, i *model.CommunityInvitation, actor int64, senderMember bool) (bool, error) {
	if !senderMember || i.Status(time.Now()) != model.CommunityInvitationPending {
		return false, nil
	}
	sender, err := s.Users.GetUserByID(ctx, i.SenderID)
	if err != nil || sender == nil {
		return false, err
	}
	blocked, err := s.Blocks.ExistsBlockRelation(ctx, i.SenderID, actor)
	return !blocked, err
}

func (s *InvitationService) Preview(ctx context.Context, id int64, token string) (*model.CommunityInvitation, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	i, err := s.load(ctx, actor, id, token)
	if err != nil {
		return nil, err
	}
	if i.Status(time.Now()) == model.CommunityInvitationPending {
		role, err := s.Members.GetRoomUserRole(ctx, i.RoomID, i.SenderID)
		if err != nil {
			return nil, err
		}
		ok, err := s.available(ctx, i, actor, role != "")
		if err != nil {
			return nil, err
		}
		if !ok {
			i.State = model.CommunityInvitationRevoked
		}
	}
	if token != "" && i.Status(time.Now()) != model.CommunityInvitationPending {
		return nil, invitationUnavailable()
	}
	return i, nil
}

func (s *InvitationService) Respond(ctx context.Context, id int64, token string, accept bool) (int64, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return 0, err
	}
	i, err := s.load(ctx, actor, id, token)
	if err != nil {
		return 0, err
	}
	room, roles, messages, err := s.withRoomMembership(ctx, i.RoomID, func(ctx context.Context, room *model.Room, roles map[int64]string) ([]*model.Message, error) {
		current, err := s.Store.Get(ctx, i.ID, true)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, invitationUnavailable()
		}
		if accept && roles[actor] != "" && current.RecipientID != nil {
			return nil, s.Store.AcceptReceived(ctx, room.ID, actor, time.Now())
		}
		if !accept && current.RecipientID != nil && current.State == model.CommunityInvitationDeclined {
			return nil, nil
		}
		ok, err := s.available(ctx, current, actor, roles[current.SenderID] != "")
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, invitationUnavailable()
		}
		if !accept {
			if current.RecipientID == nil {
				return nil, invitationUnavailable()
			}
			return nil, s.Store.SetState(ctx, current.ID, model.CommunityInvitationDeclined)
		}
		return s.join(ctx, room, roles, actor)
	})
	if err != nil {
		return 0, err
	}
	s.publish(ctx, room, roles, messages...)
	return room.ID, nil
}

func (s *InvitationService) Revoke(ctx context.Context, id int64) error {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return err
	}
	i, err := s.Store.Get(ctx, id, false)
	if err != nil {
		return err
	}
	if i == nil {
		return invitationUnavailable()
	}
	_, _, _, err = s.withRoomMembership(ctx, i.RoomID, func(ctx context.Context, _ *model.Room, roles map[int64]string) ([]*model.Message, error) {
		if roles[actor] == "" || (actor != i.SenderID && roles[actor] != model.RoomUserRoleOwner) {
			return nil, invitationUnavailable()
		}
		current, err := s.Store.Get(ctx, id, true)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, invitationUnavailable()
		}
		if current.Status(time.Now()) != model.CommunityInvitationPending {
			return nil, nil
		}
		return nil, s.Store.SetState(ctx, id, model.CommunityInvitationRevoked)
	})
	return err
}
