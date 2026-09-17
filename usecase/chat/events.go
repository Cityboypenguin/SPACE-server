package chat

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// EventPublisher は「保存に成功したあとの周辺作用」の出口（ポート）。
// PubSub 配信・未読 SSE・通知（DM / 引用返信 / メンション）・監査ログがここに入る。
//
// # ベストエフォートであること（意図的な割り切り）
//
//   - DB 保存とはトランザクション境界が別。保存はコミット済みで、ここでの失敗は
//     ログに落とすだけにし、送信・編集・削除の成否には波及させない
//     （通知が出せなかったからといって、書けたメッセージを無かったことには
//     できないため）。
//   - したがって実装は error を返さない。失敗は実装側でログに残すこと。
//   - GraphQL 型への変換が要る配信内容（PubSub に流す gqlmodel.Message など）は
//     実装側（graph パッケージのアダプタ）の責務。usecase はドメインの値だけ渡す。
//
// # Outbox を採用していない理由と、落ちたときに起きること
//
// 配信を確実にするなら、保存と同じトランザクションでイベント行を書いて別プロセスで
// 配送する（Outbox パターン）のが定石だが、今は採用していない。配送ワーカー・
// 再試行・重複配信の冪等化まで要るのに対し、ここで運ぶのはリアルタイム表示と
// 通知という「落ちても後続の操作で回復する」類のものだからである。
//
// 実際に落ちたとき何が起きるか:
//   - メッセージ自体は保存済みで消えない。再読み込みや一覧の取得では必ず出る。
//   - 出ないのは購読中の画面へのリアルタイム反映（messageAdded など）、未読バッジの
//     SSE、通知（DM / 引用返信 / メンション）。通知は再送されないので、その1件は
//     利用者に届かないまま終わる。
//   - 既読の同期が落ちた場合は、ベルの数字が実際の未読数とずれたまま残りうる。
//
// # 気づくための手がかり
//
// 失敗は必ず構造化ログに出す。握りつぶしを1つも残さないこと。
// アダプタ（graph/chat_events.go の chatEventPublisher）は logChatDelivery を通して
//
//	level=error component=chat_event_publisher delivery=<経路名> room_id=... message_id=...
//
// の形で出す。component=chat_event_publisher で絞れば取りこぼし全体が、
// delivery= でどの経路（unread_room_broadcast / dm_notification /
// message_reply_notification / mention_notification など）が落ちたかが分かる。
// 監視を足すならこのログの件数を見ること。
//
// この判断を見直す人へ: 通知が「届かないと業務が止まる」種類のものに変わったら
// （督促・課題の締切など）ベストエフォートでは足りない。そのときは Outbox を
// 入れる価値がある。ポートの形は既に「保存後に呼ばれる非同期な出口」なので、
// 実装を差し替えるだけで済むようにしてある。
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
