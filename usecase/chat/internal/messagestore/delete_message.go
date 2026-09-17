package messagestore

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// DeleteMessageUseCase はメッセージ1件の「保存処理」（論理削除）。
//
// 削除してよいか（本人・管理者・コミュニティのオーナーか、授業ルームなら学期・
// 履修が生きているか）の判定は usecase/chat の削除サービスが持ち、ここには無い
// （パッケージのコメント参照）。
type DeleteMessageUseCase interface {
	// Execute soft-deletes messageID, recording deletedBy as the actor. It returns
	// false (with no error) if the message does not exist or was already deleted.
	Execute(ctx context.Context, messageID int64, deletedBy int64) (bool, error)
}

var _ DeleteMessageUseCase = &DeleteMessageInteractor{}

type DeleteMessageInteractor struct {
	store repository.MessageStore
}

func NewDeleteMessageUseCase(store repository.MessageStore) DeleteMessageUseCase {
	return &DeleteMessageInteractor{store: store}
}

func (uc *DeleteMessageInteractor) Execute(ctx context.Context, messageID int64, deletedBy int64) (bool, error) {
	return uc.store.SoftDeleteMessage(ctx, messageID, deletedBy)
}
