// Package chat はチャット（授業内チャット・コミュニティ・DM）のアプリケーション
// サービス。チャットの業務ルールをここ1箇所に集める。
//
// ここが受け持つのは
//   - 誰がどのルームを読めるか / 書けるか（EnsureReadAccess / EnsureWriteAccess）
//   - 授業ルームの匿名ID(匿名NNN)をいつ確定させるか（投稿時）
//   - メンションをどう検証するか（usecase/message の ResolveMentions へ委譲）
//   - 保存できたあとに何を配信・通知するか（EventPublisher ポート越し）
//
// の4つで、usecase/message/* は「保存処理」だけを担う。
//
// usecase/message/* を GraphQL リゾルバから直接呼ぶと membership・学期/履修・
// ブロックの判定が丸ごと抜け落ちるため、チャットの読み書きは必ずこのサービスを
// 経由させること。リゾルバに残る仕事は「GraphQL ID のデコード」と
// 「GraphQL 型への変換」だけ。
package chat

import (
	"context"

	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	"github.com/Cityboypenguin/SPACE-server/usecase/block"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// Service はチャットの公開口。
type Service interface {
	// EnsureReadAccess は roomID を閲覧してよいかを判定し、ルームを返す。
	// メッセージだけでなく質問・回答・投票のクエリ／サブスクリプションも
	// この1つを通す（判定の重複を無くすため）。
	EnsureReadAccess(ctx context.Context, roomID int64) (*model.Room, error)
	// EnsureWriteAccess は roomID へ書き込んでよいかを判定し、ルームを返す。
	EnsureWriteAccess(ctx context.Context, roomID int64) (*model.Room, error)

	SendMessage(ctx context.Context, in SendMessageInput) (*model.Message, error)
	UpdateMessage(ctx context.Context, in UpdateMessageInput) (*model.Message, error)
	DeleteMessage(ctx context.Context, in DeleteMessageInput) (bool, error)
	ListMessages(ctx context.Context, in ListMessagesInput) (*repository.MessagePage, error)

	MarkAsRead(ctx context.Context, roomID int64) error
	GetReadStatus(ctx context.Context, roomID int64) (*roomusecase.RoomReadStatus, error)
}

// Deps はサービスが使う下位ユースケースの束。
// 合成インターフェースを1つ渡す形にすると「何に依存しているか」が読めなくなるので、
// 使うユースケースを名前付きで並べている。
type Deps struct {
	GetRoom          roomusecase.GetRoomUseCase
	GetRoomMemberIDs roomusecase.GetUserIDsByRoomIDUseCase
	GetRoomUserRole  roomusecase.GetRoomUserRoleUseCase

	// CheckRoomWritable は授業ルームの学期・履修判定（非授業ルームは素通し）。
	CheckRoomWritable  courseusecase.CheckRoomWritableUseCase
	CheckBlockRelation block.CheckBlockRelationUseCase

	GetMessage         messageusecase.GetMessageByIDUseCase
	SendMessage        messageusecase.SendMessageUseCase
	UpdateMessage      messageusecase.UpdateMessageUseCase
	DeleteMessage      messageusecase.DeleteMessageUseCase
	ListMessages       messageusecase.ListMessagesUseCase
	ListMessagesAround messageusecase.ListMessagesAroundUseCase
	ResolveMentions    messageusecase.ResolveMentionsUseCase
	ListMentions       messageusecase.ListMentionsByMessageIDsUseCase

	// GetOrCreateAnonymousIdentity は授業ルームへの投稿時に匿名IDを確定させる。
	GetOrCreateAnonymousIdentity anonusecase.GetOrCreateAnonymousIdentityUseCase

	MarkRoomAsRead          roomusecase.MarkRoomAsReadUseCase
	MarkCourseRoomAsRead    roomusecase.MarkCourseRoomAsReadUseCase
	GetRoomReadStatus       roomusecase.GetRoomReadStatusUseCase
	GetCourseRoomReadStatus roomusecase.GetCourseRoomReadStatusUseCase

	// Events は配信・通知の出口。nil を渡すと何もしない実装が入る。
	Events EventPublisher
}

var _ Service = &service{}

type service struct {
	deps Deps
}

func NewService(deps Deps) Service {
	if deps.Events == nil {
		deps.Events = NoopEventPublisher{}
	}
	return &service{deps: deps}
}

func containsInt64(values []int64, target int64) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
