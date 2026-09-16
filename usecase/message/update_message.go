package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type UpdateMessageUseCase interface {
	// mentions は ResolveMentionsUseCase で検証済みのメンション。
	// 本文を書き換えるときはメンションも貼り直す（本文から消えたメンションは行ごと消える）。
	Execute(ctx context.Context, messageID int64, updateParam model.UpdateMessageParam, mentions []*model.Mention) (*model.Message, error)
}

var _ UpdateMessageUseCase = &UpdateMessageInteractor{}

type UpdateMessageInteractor struct {
	messageRepo repository.MessageRepository
	txManager   repository.TxManager
}

func NewUpdateMessageUseCase(messageRepo repository.MessageRepository, txManager repository.TxManager) UpdateMessageUseCase {
	return &UpdateMessageInteractor{messageRepo: messageRepo, txManager: txManager}
}

func (uc *UpdateMessageInteractor) Execute(ctx context.Context, messageID int64, updateParam model.UpdateMessageParam, mentions []*model.Mention) (*model.Message, error) {
	message, err := uc.messageRepo.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, apperr.NotFound("message not found")
	}
	if updateParam.Content != nil {
		if err := validateContent(*updateParam.Content); err != nil {
			return nil, err
		}
	}
	message.UpdateMessage(updateParam)
	message.Mentions = mentions

	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		if err := uc.messageRepo.UpdateMessage(ctx, message); err != nil {
			return err
		}
		// 本文が変わらない編集ではメンションに触らない。
		if updateParam.Content == nil {
			return nil
		}
		if err := uc.messageRepo.DeleteMessageMentionsByMessageID(ctx, message.ID); err != nil {
			return err
		}
		return uc.messageRepo.CreateMessageMentions(ctx, message.ID, mentions)
	}); err != nil {
		return nil, err
	}
	return message, nil
}
