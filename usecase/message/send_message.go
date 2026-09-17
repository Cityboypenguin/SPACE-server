package message

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// SendMessageUseCase はメッセージ1件の「保存処理」。
//
// 権限判定はここには無い。授業ルームの学期・履修、room_users の membership、
// DM のブロック判定、匿名IDの採番は全て usecase/chat の ChatService が持つので、
// このユースケースは必ずサービス経由で呼ぶこと（直接呼ぶとそれらが丸ごと
// 飛ばされる）。
type SendMessageUseCase interface {
	// replyToID を渡すと引用返信になる。返信先は同じルームの未削除メッセージである
	// 必要があり、そうでなければエラーになる。
	// mentions は ResolveMentionsUseCase で検証済みのメンション（コミュニティのみ）。
	Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []model.MediaInput, replyToID *int64, mentions []*model.Mention) (*model.Message, error)
}

var _ SendMessageUseCase = &SendMessageInteractor{}

// メッセージ本体とメンション行は関心が違うので、それぞれの狭い口を受け取る。
type SendMessageInteractor struct {
	store        repository.MessageStore
	mentionStore repository.MessageMentionStore
	mediaRepo    repository.MediaRepository
	txManager    repository.TxManager
}

func NewSendMessageUseCase(
	store repository.MessageStore,
	mentionStore repository.MessageMentionStore,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
) SendMessageUseCase {
	return &SendMessageInteractor{
		store:        store,
		mentionStore: mentionStore,
		mediaRepo:    mediaRepo,
		txManager:    txManager,
	}
}

func (uc *SendMessageInteractor) Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []model.MediaInput, replyToID *int64, mentions []*model.Mention) (*model.Message, error) {
	content = strings.TrimSpace(content)
	if err := validateContent(content); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("media/%d/", userID)
	mediaKeys := make([]string, 0, len(mediaInputs))
	for _, input := range mediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
		mediaKeys = append(mediaKeys, input.StorageKey)
	}

	// 返信先は同じルームの、まだ削除されていないメッセージだけ許す。
	// （他ルームのメッセージIDを指定して内容を覗き見られるのを防ぐ）
	if replyToID != nil {
		parent, err := uc.store.GetMessageByID(ctx, *replyToID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, apperr.InvalidInput("返信先のメッセージが見つかりません")
		}
		if !parent.IsInRoom(roomID) {
			return nil, apperr.InvalidInput("返信先のメッセージが見つかりません")
		}
	}

	now := time.Now()
	// 「本文も添付も空」の検証は model.NewMessage が持つ（生成経路が増えても漏れないように）。
	m, err := model.NewMessage(model.CreateMessageParam{
		RoomID:    roomID,
		UserID:    userID,
		ReplyToID: replyToID,
		Content:   content,
		MediaKeys: mediaKeys,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	m.Mentions = mentions

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.store.SaveMessage(ctx, m); err != nil {
			return err
		}
		if err := uc.mentionStore.CreateMessageMentions(ctx, m.ID, mentions); err != nil {
			return err
		}
		for i, input := range mediaInputs {
			media := model.NewMedia(userID, input, now)
			if err := uc.mediaRepo.CreateMedia(ctx, media); err != nil {
				return err
			}
			if err := uc.mediaRepo.CreateMessageMedia(ctx, m.ID, media.ID, i); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return m, nil
}
