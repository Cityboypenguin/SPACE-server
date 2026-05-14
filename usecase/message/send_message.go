package message

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MediaInput struct {
	StorageKey  string
	ContentType string
}

type SendMessageUseCase interface {
	Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []MediaInput) (*model.Message, error)
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

func (uc *SendMessageInteractor) Execute(ctx context.Context, roomID, userID int64, content string, mediaInputs []MediaInput) (*model.Message, error) {
	if content == "" && len(mediaInputs) == 0 {
		return nil, fmt.Errorf("content or media is required")
	}

	prefix := fmt.Sprintf("media/%d/", userID)
	for _, input := range mediaInputs {
		if !strings.HasPrefix(input.StorageKey, prefix) {
			return nil, fmt.Errorf("invalid media key")
		}
	}

	now := time.Now()
	m := &model.Message{}
	m.CreateMessage(model.CreateMessageParam{
		RoomID:    roomID,
		UserID:    userID,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	})

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.messageRepo.SaveMessage(ctx, m); err != nil {
			return err
		}
		for i, input := range mediaInputs {
			media := &model.Media{
				UploaderUserID: userID,
				StorageKey:     input.StorageKey,
				ContentType:    input.ContentType,
				CreatedAt:      now,
			}
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
