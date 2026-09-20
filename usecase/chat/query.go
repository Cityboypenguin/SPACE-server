package chat

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
)

// MessageQueryService はメッセージの読み取り。
//
// 読み取りだけなので依存は「閲覧してよいかの判定」と「取得の2経路」の3つで済む。
// 書き込み経路と同じサービスに同居させていたときは、一覧を出すだけの呼び出しが
// 保存処理や通知の依存まで引き連れていた。
type MessageQueryService interface {
	ListMessages(ctx context.Context, in ListMessagesInput) (*repository.MessagePage, error)
}

// ListMessagesInput はメッセージ一覧の取得条件。AroundID を指定すると
// そのメッセージを中心に前後を取り、Cursor は無視される。
type ListMessagesInput struct {
	RoomID   int64
	Limit    int
	Cursor   repository.MessageCursor
	AroundID *int64
}

// MessageQueryDeps は一覧取得が実際に使うものだけ。
type MessageQueryDeps struct {
	// Access は閲覧権限の判定。ここを飛ばすと非メンバーが他人のルームを読めてしまう。
	Access AccessPolicy

	ListMessages       messageusecase.ListMessagesUseCase
	ListMessagesAround messageusecase.ListMessagesAroundUseCase
}

var _ MessageQueryService = &messageQueryService{}

type messageQueryService struct {
	deps MessageQueryDeps
}

func NewMessageQueryService(deps MessageQueryDeps) MessageQueryService {
	return &messageQueryService{deps: deps}
}

func (s *messageQueryService) ListMessages(ctx context.Context, in ListMessagesInput) (*repository.MessagePage, error) {
	if _, err := s.deps.Access.EnsureReadAccess(ctx, in.RoomID); err != nil {
		return nil, err
	}

	if in.AroundID != nil {
		return s.deps.ListMessagesAround.Execute(ctx, in.RoomID, *in.AroundID, in.Limit)
	}
	return s.deps.ListMessages.Execute(ctx, repository.MessageQuery{
		RoomID: in.RoomID,
		Limit:  in.Limit,
		Cursor: in.Cursor,
	})
}
