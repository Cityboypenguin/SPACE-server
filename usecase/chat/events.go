package chat

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// EventPublisher は「保存に成功したあとの周辺作用」の出口（ポート）。
// PubSub 配信・未読 SSE・通知（DM / 引用返信 / メンション）・監査ログがここに入る。
//
// 設計上の割り切り:
//   - DB 保存とはトランザクション境界が別。保存はコミット済みで、ここでの失敗は
//     ログに落とすだけにし、送信・編集・削除の成否には波及させない
//     （通知が出せなかったからといって、書けたメッセージを無かったことには
//     できないため）。
//   - したがって実装は error を返さない。失敗は実装側でログに残すこと。
//   - GraphQL 型への変換が要る配信内容（PubSub に流す gqlmodel.Message など）は
//     実装側（graph パッケージのアダプタ）の責務。usecase はドメインの値だけ渡す。
type EventPublisher interface {
	MessageSent(ctx context.Context, ev MessageSentEvent)
	MessageUpdated(ctx context.Context, ev MessageUpdatedEvent)
	MessageDeleted(ctx context.Context, ev MessageDeletedEvent)
	RoomMarkedAsRead(ctx context.Context, ev RoomMarkedAsReadEvent)
}

// MessageSentEvent はメッセージ1件が保存できたこと。
type MessageSentEvent struct {
	Room    *model.Room
	Message *model.Message
	ActorID int64
	// MemberIDs は非授業ルームのメンバー。DM 通知の宛先に使う。
	// 授業ルームは room_users を使わないため nil。
	MemberIDs []int64
	// HasMedia は添付があったか。本文が空のときのプレビュー代替文言に使う。
	HasMedia bool
}

// MessageUpdatedEvent はメッセージ1件の編集が保存できたこと。
type MessageUpdatedEvent struct {
	Room    *model.Room
	Message *model.Message
	// ActorID は通知の「誰が」。編集では元の投稿者を入れる（管理者が代理編集しても
	// メンションされた側から見た相手は投稿者のままにするため）。
	ActorID int64
	// AddedMentions は編集で新しく足されたメンションだけ。既にメンション済みの
	// 相手へ再通知しないよう、サービス側で差分を取ってある。
	AddedMentions []*model.Mention
}

// MessageDeletedEvent はメッセージ1件の論理削除ができたこと。
// RoomID は引数ではなくメッセージ実体のものなので、配信先の topic を取り違えない。
type MessageDeletedEvent struct {
	RoomID    int64
	MessageID int64
}

// RoomMarkedAsReadEvent はルームを既読にしたこと。
type RoomMarkedAsReadEvent struct {
	Room    *model.Room
	ActorID int64
	// MemberIDs は非授業ルームのメンバー（DM 通知の既読化に使う）。授業ルームは nil。
	MemberIDs []int64
}

// NoopEventPublisher は何も配信しない実装。テストや、配信先を組み立てていない
// 起動経路（バッチなど）で使う。
type NoopEventPublisher struct{}

var _ EventPublisher = NoopEventPublisher{}

func (NoopEventPublisher) MessageSent(context.Context, MessageSentEvent)           {}
func (NoopEventPublisher) MessageUpdated(context.Context, MessageUpdatedEvent)     {}
func (NoopEventPublisher) MessageDeleted(context.Context, MessageDeletedEvent)     {}
func (NoopEventPublisher) RoomMarkedAsRead(context.Context, RoomMarkedAsReadEvent) {}
