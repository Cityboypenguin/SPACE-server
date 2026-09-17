package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// UpdateMessageUseCase はメッセージ1件の「保存処理」（本文とメンションの貼り直し）。
//
// 編集してよいか（本人か、授業ルームなら学期・履修が生きているか）の判定は
// usecase/chat の ChatService が持つ。必ずサービス経由で呼ぶこと。
type UpdateMessageUseCase interface {
	// mentions は ResolveMentionsUseCase で検証済みのメンション。
	// 本文を書き換えるときはメンションも貼り直す（本文から消えたメンションは行ごと消える）。
	Execute(ctx context.Context, messageID int64, updateParam model.UpdateMessageParam, mentions []*model.Mention) (*model.Message, error)
}

var _ UpdateMessageUseCase = &UpdateMessageInteractor{}

type UpdateMessageInteractor struct {
	store        repository.MessageStore
	mentionStore repository.MessageMentionStore
	txManager    repository.TxManager
}

func NewUpdateMessageUseCase(
	store repository.MessageStore,
	mentionStore repository.MessageMentionStore,
	txManager repository.TxManager,
) UpdateMessageUseCase {
	return &UpdateMessageInteractor{store: store, mentionStore: mentionStore, txManager: txManager}
}

func (uc *UpdateMessageInteractor) Execute(ctx context.Context, messageID int64, updateParam model.UpdateMessageParam, mentions []*model.Mention) (*model.Message, error) {
	message, err := uc.store.GetMessageByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if message == nil || message.IsDeleted() {
		// GetMessageByID は削除済みを返さないが、削除済みは編集不可という
		// 不変条件を model 側と揃えて明示しておく。
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
		if err := uc.store.UpdateMessage(ctx, message); err != nil {
			return err
		}
		// 本文が変わらない編集ではメンションに触らない。
		if updateParam.Content == nil {
			return nil
		}
		if err := uc.mentionStore.DeleteMessageMentionsByMessageID(ctx, message.ID); err != nil {
			return err
		}
		return uc.mentionStore.CreateMessageMentions(ctx, message.ID, mentions)
	}); err != nil {
		return nil, err
	}
	return message, nil
}
