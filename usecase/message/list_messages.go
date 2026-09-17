package message

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ListMessagesUseCase はルームのメッセージを1ページ返す。
//
// カーソルの意味（どれか1つだけ指定する。複数指定時は AfterTime > AfterID > BeforeID の順で採用）:
//   - 指定なし     : 最新ページ。新しい方から limit 件（返す順は古い順）。
//   - BeforeID     : そのID未満＝さらに過去へ遡るとき。上スクロールの追加読み込み。
//   - AfterID      : そのIDより新しい＝取りこぼしを埋めるとき（再接続時など）。
//   - AfterTime    : その時刻より後＝未読位置から読み直すとき。
//
// 戻り値の HasMoreBefore / HasMoreAfter は「このページの前後に未削除の
// メッセージが実際に存在するか」で、推定ではなく毎回実測している。
// 0件だった場合はカーソルの位置を基準に判定するので、部屋の先頭・末尾でも
// 嘘の "もっとある" を返さない。
type ListMessagesUseCase interface {
	Execute(ctx context.Context, query repository.MessageQuery) (*repository.MessagePage, error)
}

var _ ListMessagesUseCase = &ListMessagesInteractor{}

type ListMessagesInteractor struct {
	readModel repository.MessageReadModel
}

func NewListMessagesUseCase(readModel repository.MessageReadModel) ListMessagesUseCase {
	return &ListMessagesInteractor{readModel: readModel}
}

func (uc *ListMessagesInteractor) Execute(ctx context.Context, query repository.MessageQuery) (*repository.MessagePage, error) {
	return uc.readModel.ListMessagesByRoomID(ctx, query)
}
