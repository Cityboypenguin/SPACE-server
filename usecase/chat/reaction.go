package chat

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// MessageReactionService はメッセージへのリアクション（絵文字）の付け外し。
//
// 付けてよいのは model.AllowedMessageReactionEmojis の絵文字だけ。権限は編集・削除と
// 同じ ensureMutateAccess を通す（授業内チャットなら現学期かつ履修中、コミュニティ・DM
// なら参加者）。他人のメッセージにも付けられるので、所有権の判定は無い。
type MessageReactionService interface {
	AddReaction(ctx context.Context, in MessageReactionInput) (*model.Message, error)
	RemoveReaction(ctx context.Context, in MessageReactionInput) (*model.Message, error)
}

// maxReactionEmojiRunes は message_reactions.emoji（VARCHAR(32)）に入る最大の文字数。
const maxReactionEmojiRunes = 32

type MessageReactionInput struct {
	RoomID    int64
	MessageID int64
	Emoji     string
}

type MessageReactionDeps struct {
	// Access は権限判定。自前で membership を見に行かないこと。
	Access AccessPolicy
	// GetRoom は「メッセージが実際に属するルーム」を引くため（呼び出し元が名乗った
	// roomID ではなく実体を使う。loadMessageInRoom 参照）。
	GetRoom    roomusecase.GetRoomUseCase
	GetMessage messageusecase.GetMessageByIDUseCase
	Reactions  repository.MessageReactionRepository
	// Events は配信の出口。nil を渡すと何もしない実装が入る。
	Events EventPublisher
}

var _ MessageReactionService = &messageReactionService{}

type messageReactionService struct {
	deps MessageReactionDeps
}

func NewMessageReactionService(deps MessageReactionDeps) MessageReactionService {
	if deps.Events == nil {
		deps.Events = NoopEventPublisher{}
	}
	return &messageReactionService{deps: deps}
}

func (s *messageReactionService) AddReaction(ctx context.Context, in MessageReactionInput) (*model.Message, error) {
	return s.change(ctx, in, true, s.deps.Reactions.AddMessageReaction)
}

func (s *messageReactionService) RemoveReaction(ctx context.Context, in MessageReactionInput) (*model.Message, error) {
	// 外すほうは許可リストを見ない。一覧から絵文字を抜いたあとも、すでに付いている
	// リアクションを本人が外せるようにするため（外す対象は自分の行だけなので害はない）。
	return s.change(ctx, in, false, s.deps.Reactions.RemoveMessageReaction)
}

// change は付ける・外すに共通の流れ（認証 → 絵文字の検証 → メッセージの照合 →
// 権限判定 → 保存 → 配信）。どちらも冪等なので、既に付いている／付いていない状態への
// 操作もエラーにはせず、そのときの最新の状態を配信して返す。
//
// requireAllowed は絵文字が許可リストにあることを求めるか。付けるときだけ true にする。
// 外すときに求めないのは、一覧から絵文字を抜いたあとも、すでに付いているリアクションを
// 本人が外せるようにするため（外す対象は自分の行だけなので害はない）。
func (s *messageReactionService) change(
	ctx context.Context,
	in MessageReactionInput,
	requireAllowed bool,
	apply func(ctx context.Context, messageID, userID int64, emoji string) error,
) (*model.Message, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if requireAllowed {
		if !model.IsAllowedMessageReactionEmoji(in.Emoji) {
			return nil, apperr.InvalidInput("この絵文字は使えません")
		}
	} else if in.Emoji == "" || utf8.RuneCountInString(in.Emoji) > maxReactionEmojiRunes {
		// 外すときは許可リストを見ないが、保存形式（VARCHAR(32)）を超える長い文字列が
		// DB まで届かないよう、長さだけは見る。
		return nil, apperr.InvalidInput("この絵文字は使えません")
	}

	msg, err := loadMessageInRoom(ctx, s.deps.GetMessage, in.RoomID, in.MessageID)
	if err != nil {
		return nil, err
	}

	room, err := s.deps.GetRoom.Execute(ctx, msg.RoomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room")
	}
	if err := s.deps.Access.ensureMutateAccess(ctx, claims, room); err != nil {
		return nil, err
	}

	if err := apply(ctx, msg.ID, claims.ID, in.Emoji); err != nil {
		return nil, err
	}

	// ActorID は通知の「誰が」だが、AddedMentions が空なので通知は出ない。編集と同じく
	// 元の投稿者を入れておく（リアクションした人を、配信の宛先や通知に漏らさない）。
	s.deps.Events.MessageUpdated(ctx, MessageUpdatedEvent{
		Room:    room,
		Message: msg,
		ActorID: msg.UserID,
	})
	return msg, nil
}
