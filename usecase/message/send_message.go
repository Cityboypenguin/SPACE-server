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

type SendMessageUseCase interface {
	// replyToID を渡すと引用返信になる。返信先は同じルームの未削除メッセージである
	// 必要があり、そうでなければエラーになる。
	// mentions は ResolveMentionsUseCase で検証済みのメンション（コミュニティのみ）。
	Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []model.MediaInput, replyToID *int64, mentions []*model.Mention) (*model.Message, error)
}

var _ SendMessageUseCase = &SendMessageInteractor{}

type SendMessageInteractor struct {
	messageRepo repository.MessageRepository
	mediaRepo   repository.MediaRepository
	txManager   repository.TxManager
}

func NewSendMessageUseCase(
	messageRepo repository.MessageRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
) SendMessageUseCase {
	return &SendMessageInteractor{
		messageRepo: messageRepo,
		mediaRepo:   mediaRepo,
		txManager:   txManager,
	}
}

func (uc *SendMessageInteractor) Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []model.MediaInput, replyToID *int64, mentions []*model.Mention) (*model.Message, error) {
	content = strings.TrimSpace(content)
	if content == "" && len(mediaInputs) == 0 {
		return nil, apperr.InvalidInput("content or media is required")
	}
	if err := validateContent(content); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("media/%d/", userID)
	for _, input := range mediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
	}

	// 返信先は同じルームの、まだ削除されていないメッセージだけ許す。
	// （他ルームのメッセージIDを指定して内容を覗き見られるのを防ぐ）
	if replyToID != nil {
		parent, err := uc.messageRepo.GetMessageByID(ctx, *replyToID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, apperr.InvalidInput("返信先のメッセージが見つかりません")
		}
		if parent.RoomID != roomID {
			return nil, apperr.InvalidInput("返信先のメッセージが見つかりません")
		}
	}

	now := time.Now()
	m := &model.Message{Mentions: mentions}
	m.CreateMessage(model.CreateMessageParam{
		RoomID:    roomID,
		UserID:    userID,
		ReplyToID: replyToID,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	})

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.messageRepo.SaveMessage(ctx, m); err != nil {
			return err
		}
		if err := uc.messageRepo.CreateMessageMentions(ctx, m.ID, mentions); err != nil {
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
