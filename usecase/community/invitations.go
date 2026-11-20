package community

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/chat"
	"github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

const invitationLifetime = 7 * 24 * time.Hour
const invitationLimitPerHour = 20

// InvitationBatchLimit は一度にまとめて招待できる人数。
const InvitationBatchLimit = 20

// invitationLifetimes は招待リンクの有効期間として選べる値。
var invitationLifetimes = []time.Duration{time.Hour, 24 * time.Hour, 3 * 24 * time.Hour, invitationLifetime, 30 * 24 * time.Hour}

// EventCommunityInvitationsChanged は「招待の一覧が変わった」ことだけを伝える SSE のイベント名。
// 中身は載せず、受け取った画面が自分で取り直す。
const EventCommunityInvitationsChanged = "community_invitations_changed"

type invitationStore interface {
	Create(context.Context, *model.CommunityInvitation, []byte) error
	Get(context.Context, int64, bool) (*model.CommunityInvitation, error)
	GetByToken(context.Context, []byte) (*model.CommunityInvitation, error)
	Pending(context.Context, int64, int64, time.Time) (*model.CommunityInvitation, error)
	ListReceived(context.Context, int64, time.Time, int32, int32) ([]*model.CommunityInvitation, int32, error)
	ListSent(context.Context, int64, int64, bool, bool, time.Time, int32, int32) ([]*model.CommunityInvitation, error)
	CandidateStates(context.Context, int64, int64, []int64, time.Time) (map[int64]string, error)
	RecordUse(context.Context, int64, int64, time.Time) error
	RecentCount(context.Context, int64, int64, time.Time) (int, error)
	RecentlyDeclined(context.Context, int64, int64, time.Time) (bool, error)
	SetState(context.Context, int64, string) error
	AcceptReceived(context.Context, int64, int64, time.Time) error
	JoinStampSettings(context.Context) (model.CommunityJoinStampSettings, error)
	SaveJoinStampSettings(context.Context, model.CommunityJoinStampSettings) error
	WelcomeTarget(context.Context, int64, int64) (*model.CommunityWelcomeTarget, error)
	SetWelcomeMessage(context.Context, int64, int64, int64) error
	MarkWelcomeSent(context.Context, int64, int64) error
	JoinGreetingPending(context.Context, int64, int64) (bool, error)
	MarkJoinGreetingDone(context.Context, int64, int64) (bool, error)
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
	// Delivery は招待の一覧を開いている画面へ「変わった」と知らせる口。nil なら知らせない。
	Delivery notification.UserEventDelivery
}

// signal は招待の一覧が変わったことを本人たちへ知らせる（ベストエフォート）。
func (s *InvitationService) signal(roomID int64, userIDs ...int64) {
	if s.Delivery == nil {
		return
	}
	seen := make(map[int64]bool, len(userIDs))
	for _, id := range userIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		s.Delivery.PublishToUser(id, EventCommunityInvitationsChanged, map[string]any{"roomID": opaqueid.Encode("room", roomID)})
	}
}

// normalizeInviteOptions は入力を検証し、空のメッセージを nil にそろえる。
func normalizeInviteOptions(opts model.CommunityInviteOptions) (model.CommunityInviteOptions, error) {
	if opts.Message != nil {
		message := strings.TrimSpace(*opts.Message)
		if utf8.RuneCountInString(message) > model.CommunityInvitationMessageMaxLength {
			return opts, apperr.InvalidInput("メッセージは200文字以内で入力してください")
		}
		if message == "" {
			opts.Message = nil
		} else {
			opts.Message = &message
		}
	}
	if opts.ExpiresIn == 0 {
		opts.ExpiresIn = invitationLifetime
	} else if !slices.Contains(invitationLifetimes, opts.ExpiresIn) {
		return opts, apperr.InvalidInput("有効期限の指定が不正です")
	}
	return opts, nil
}

func invitationUnavailable() error {
	return apperr.NotFound("この招待は利用できません")
}

func (s *InvitationService) Send(ctx context.Context, communityID int64, recipientID *int64, opts model.CommunityInviteOptions) (*model.CommunityInvitation, string, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, "", err
	}
	opts, err = normalizeInviteOptions(opts)
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
		State: model.CommunityInvitationPending, Message: opts.Message,
		CreatedAt: now, ExpiresAt: now.Add(opts.ExpiresIn)}
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
		s.signal(c.RoomID, *recipientID)
	}
	return i, token, nil
}

// InviteResult はまとめて招待したときの1人ぶんの結果。Err が nil なら送れた。
type InviteResult struct {
	RecipientID int64
	Invitation  *model.CommunityInvitation
	Err         error
}

// SendMany は複数の人へ同じ内容で招待を送る。1人の失敗で他の人の招待は止めない。
// 上限に達したら、残りの人には同じ理由の失敗を返す。
func (s *InvitationService) SendMany(ctx context.Context, communityID int64, recipientIDs []int64, opts model.CommunityInviteOptions) ([]InviteResult, error) {
	if len(recipientIDs) == 0 {
		return nil, apperr.InvalidInput("招待する人を選んでください")
	}
	if len(recipientIDs) > InvitationBatchLimit {
		return nil, apperr.InvalidInput("一度に招待できるのは20人までです")
	}
	if _, err := normalizeInviteOptions(opts); err != nil {
		return nil, err
	}
	results := make([]InviteResult, 0, len(recipientIDs))
	seen := make(map[int64]bool, len(recipientIDs))
	for _, id := range recipientIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		recipient := id
		i, _, err := s.Send(ctx, communityID, &recipient, opts)
		var appErr *apperr.Error
		if err != nil && !errors.As(err, &appErr) {
			// 想定外の失敗（DB 障害など）は、まとめて失敗として返す。
			return nil, err
		}
		results = append(results, InviteResult{RecipientID: id, Invitation: i, Err: err})
	}
	return results, nil
}

// InviteCandidate は招待の候補（検索結果の1人）とその状態。
type InviteCandidate struct {
	User  *model.User
	State string
}

// Candidates は検索結果の人それぞれが、このコミュニティへ招待できるかを返す。
func (s *InvitationService) Candidates(ctx context.Context, communityID int64, users []*model.User) ([]InviteCandidate, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.Communities.GetCommunityByID(ctx, communityID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, apperr.NotFound("コミュニティが見つかりません")
	}
	role, err := s.Members.GetRoomUserRole(ctx, c.RoomID, actor)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, apperr.Forbidden("コミュニティのメンバーのみ招待できます")
	}
	ids := make([]int64, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	states, err := s.Store.CandidateStates(ctx, c.RoomID, actor, ids, time.Now())
	if err != nil {
		return nil, err
	}
	result := make([]InviteCandidate, 0, len(users))
	for _, user := range users {
		state := states[user.ID]
		if state == "" {
			state = model.CommunityCandidateAvailable
		}
		result = append(result, InviteCandidate{User: user, State: state})
	}
	return result, nil
}

func (s *InvitationService) Received(ctx context.Context, limit, offset int32) ([]*model.CommunityInvitation, int32, error) {
	actor, err := authz.CallerID(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.Store.ListReceived(ctx, actor, time.Now(), limit, offset)
}

func (s *InvitationService) Sent(ctx context.Context, roomID int64, history bool, limit, offset int32) ([]*model.CommunityInvitation, error) {
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
	return s.Store.ListSent(ctx, roomID, actor, role == model.RoomUserRoleOwner, history, time.Now(), limit, offset)
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
	var used *model.CommunityInvitation // 参加に使われた招待（参加しなかったら nil）
	declined := false
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
			declined = true
			return nil, s.Store.SetState(ctx, current.ID, model.CommunityInvitationDeclined)
		}
		wasMember := roles[actor] != ""
		messages, err := s.join(ctx, room, roles, actor)
		if err != nil || wasMember {
			return messages, err
		}
		if err := s.Store.RecordUse(ctx, current.ID, actor, time.Now()); err != nil {
			return nil, err
		}
		used = current
		return messages, nil
	})
	if err != nil {
		return 0, err
	}
	s.publish(ctx, room, roles, messages...)
	switch {
	case used != nil:
		s.notifyAccepted(ctx, room, used, actor)
		s.signal(room.ID, used.SenderID, actor)
	case declined:
		s.signal(room.ID, i.SenderID, actor)
	}
	return room.ID, nil
}

// notifyAccepted は招待を作った人へ「招待から参加した」ことを知らせる（ベストエフォート）。
func (s *InvitationService) notifyAccepted(ctx context.Context, room *model.Room, used *model.CommunityInvitation, actor int64) {
	if used.SenderID == actor {
		return
	}
	user, err := s.Users.GetUserByID(ctx, actor)
	if err != nil || user == nil {
		return
	}
	name := room.Name
	if communityID, err := s.Communities.GetCommunityIDByRoomID(ctx, room.ID); err == nil && communityID != 0 {
		if c, err := s.Communities.GetCommunityByID(ctx, communityID); err == nil && c != nil {
			name = c.Name
		}
	}
	via := "招待"
	if used.RecipientID == nil {
		via = "招待リンク"
	}
	target := notification.TargetRoom
	err = s.Notifications.Publish(ctx, notification.PublishParams{
		UserID: used.SenderID, ActorID: &actor, TargetType: &target, TargetID: &room.ID,
		Type:    notification.TypeCommunityInvitationAccepted,
		Message: user.Name + "さんが" + via + "から" + name + "に参加しました",
	})
	if err != nil {
		logger.Log.Error().Err(err).Int64("invitation_id", used.ID).Msg("community invitation accepted notification failed")
	}
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
	if err != nil {
		return err
	}
	if i.RecipientID != nil {
		s.signal(i.RoomID, *i.RecipientID, i.SenderID, actor)
	} else {
		s.signal(i.RoomID, i.SenderID, actor)
	}
	return nil
}
