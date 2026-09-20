package graph

import (
	"context"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// chatEventPubSub / chatEventSSEBroker は配信先の狭い口。
//
// *pubsub.PubSub / *sse.Broker をそのまま持つとアダプタ単体のテストが書けないので、
// 実際に呼ぶメソッドだけのインターフェースを切って受け取る（実装側は既存の型が
// そのまま満たすので、本番の配線は何も変わらない）。
type chatEventPubSub interface {
	Publish(topic string, data interface{})
}

type chatEventSSEBroker interface {
	PublishToUser(userID int64, eventType string, data map[string]any)
	PublishNotificationsChangedToUser(userID int64)
}

// chatEventAsyncRunner は「順序も応答時間も要らない配信」をリクエストの外へ出す口。
//
// 本番では *chatusecase.AsyncRunner が入る。インターフェースにしてあるのは、
// テストがその場で実行する runner を差し込んで、非同期かどうかと関係なく
// 「何をどこへ配信したか」を確かめられるようにするため
// （非同期化そのものは usecase/chat/async_events_test.go の担当）。
type chatEventAsyncRunner interface {
	Go(ctx context.Context, name string, fn func(context.Context))
}

// ChatEventPublisherDeps は配信・通知アダプタが使う依存。
//
// 以前は *Resolver をまるごと持っていたため、(a) 何に依存しているかがコードから
// 読めず、(b) アダプタ単体のテストに巨大な Resolver の組み立てが要り、(c) main.go
// が「resolver を作る→resolver にサービスを差す」という相互参照になっていた。
// 実際に使うものだけを名前付きで受け取れば、配線は publisher → サービス → resolver の
// 一方向で済む。
type ChatEventPublisherDeps struct {
	PubSub                chatEventPubSub
	SSEBroker             chatEventSSEBroker
	NotificationPublisher notificationuc.NotificationPublisher

	// CourseRegistrantIDs は授業ルームの room_changed の宛先（履修者のID）。
	// 授業ルームは room_users を使わないため、宛先は時間割から引く
	// （非授業ルームの宛先は既にイベントに載っている。roomChangedRecipients 参照）。
	CourseRegistrantIDs roomusecase.GetCourseRegistrantIDsUseCase

	// Async は順序を要しない配信（room_changed の SSE と各種通知）の逃がし先。
	// nil なら全てその場で同期実行する（テストや配信先を組み立てない起動経路向け）。
	Async chatEventAsyncRunner

	// GetMessage は引用返信の通知先（返信元の投稿者）を引くため。
	GetMessage messageusecase.GetMessageByIDUseCase
	// GetAnonymousIdentity は授業ルームの返信通知の文言に使う匿名ラベル。
	// 採番しない読み取り専用の口（採番は投稿時に送信サービス側で済んでいる）。
	GetAnonymousIdentity anonusecase.GetAnonymousIdentityUseCase

	MarkNotificationsAsReadByActor notificationuc.MarkAllAsReadByActorUseCase
}

// chatEventPublisher は chat.EventPublisher の実装。PubSub / SSE / 通知 /
// 監査ログという「GraphQL 側だけが知っている配信先」をまとめて引き受ける。
//
// ここに置いているのは、配信内容の組み立てに GraphQL 型（gqlmodel.Message）や
// 不透明ID（opaqueid）が要るため。usecase/chat はドメインの値だけを渡してくる。
//
// 同期と非同期の切り分けはこのアダプタが持つ（外側で一律に非同期化しない）。
// 購読中の画面へ流す PubSub は順序が崩れるとチャット本体の並びが壊れるのでリクエストの
// 中で同期に、room_changed の SSE と通知は宛先ぶんの時間がかかり順序も要らないので
// deps.Async へ逃がす。理由は usecase/chat/async_events.go のコメントに詳しい。
//
// ポートの契約どおり、どのメソッドも error を返さない。保存は既にコミット済みで、
// 配信の失敗でメッセージ送信を失敗させるわけにはいかないので、失敗はログに残して
// 先へ進む。ログは全て logChatDelivery を通し、あとから追える形に揃えている
// （usecase/chat/events.go のポートのコメントを参照）。
type chatEventPublisher struct {
	deps ChatEventPublisherDeps
}

var _ chatusecase.EventPublisher = &chatEventPublisher{}

// NewChatEventPublisher はチャットサービスのイベント出口を、実際の配信先へ繋ぐ。
func NewChatEventPublisher(deps ChatEventPublisherDeps) chatusecase.EventPublisher {
	return &chatEventPublisher{deps: deps}
}

// chatEventRoomChanged は「このルームが更新された」ことを知らせる SSE のイベント名。
//
// 以前の unread_room から改名してある。理由は broadcastRoomChanged のコメント参照
// （ペイロードから unreadCount が消えるので、名前を据え置くと古いクライアントが
// 壊れた数字を表示しうる）。古い unread_room はもう一切送らない。
const chatEventRoomChanged = "room_changed"

// 配信・通知の失敗ログで使う stage 名。ログを grep する側が「どの経路が落ちたか」で
// 絞り込めるよう、文字列はここ1箇所に集める。
const (
	chatDeliveryRoomChanged       = "room_changed_broadcast"
	chatDeliveryDMNotification    = "dm_notification"
	chatDeliveryReplyNotification = "message_reply_notification"
	chatDeliveryReplyParentLookup = "message_reply_parent_lookup"
	chatDeliveryAnonymousLabel    = "anonymous_label_lookup"
	chatDeliveryMentionNotify     = "mention_notification"
	chatDeliveryDMReadSync        = "dm_notification_read_sync"
)

// logChatDelivery はベストエフォートな配信・通知の取りこぼしを、必ず同じ形で残す。
//
// Outbox を持たない以上、ここに出ないと「メッセージは保存されたのに通知だけ出て
// いない」状態に誰も気づけない。呼び出し側は room_id / message_id など後から
// 追える情報を足してから Msg すること。
func logChatDelivery(err error, stage string) *zerolog.Event {
	return logger.Log.Error().Err(err).
		Str("component", "chat_event_publisher").
		Str("delivery", stage)
}

func (p *chatEventPublisher) MessageSent(ctx context.Context, ev chatusecase.MessageSentEvent) {
	roomGraphID := encodeGraphID("room", ev.Room.ID)

	// 購読中の画面へのメッセージ配信だけは同期。インメモリの publish なので安価な一方、
	// 順序が入れ替わるとチャット本体のメッセージが入れ替わって表示されるため。
	p.deps.PubSub.Publish(roomGraphID+":message:added", toGraphMessage(ev.Message))

	// 残り（宛先ぶんの SSE と通知）はリクエストの外へ。どちらも順序を要しない。
	p.async(ctx, "message_sent", func(ctx context.Context) {
		p.broadcastRoomChanged(ctx, ev, roomGraphID)
		p.publishSentNotifications(ctx, ev)
	})
}

// broadcastRoomChanged は「このルームが更新された」という事実だけを宛先へ配る。
//
// # なぜ未読数を載せないのか
//
// 以前は送信のたびにルームの全メンバー・全履修者ぶんの未読数を集計して各人へ配って
// いた。派生値をサーバが全員ぶん計算して配る形で、(a) 履修者数百人規模の授業では
// 投稿1件ごとに履修者×メッセージの JOIN が走り、(b) 接続していない利用者のぶんまで
// 数え、(c) 受け取ったクライアントはどのみち自分の未読数を取り直していた。
// 数えるのをやめ、未読数は「必要になった利用者が自分ぶんだけ取る」ことにした。
//
// # なぜイベント名を unread_room から room_changed へ変えたのか
//
// ペイロードから unreadCount が消えるので、名前を据え置くとデプロイ中の古い
// クライアントが unreadCount: undefined を一覧の行へ書き込み、壊れた数字を表示しうる。
// 名前を変えれば古いクライアントはこのイベントを受け取らず、「次の取得まで未読数が
// 古いまま」という安全な劣化で済む。古い unread_room はもう送らない（集計を消したので
// 送りようがない）。
//
// # なぜ actorID を載せないのか
//
// 授業内チャットは匿名なので、投稿者を特定できる情報をイベントに乗せると匿名性が崩れる
// （受け取った側が roomID と突き合わせれば「今その授業に投稿したのは誰か」が分かる）。
// 代わりに配信対象から投稿者自身を除外し、hasNewMessage は常に true にする。
// 受信者ごとに値を変える必要が無くなるので、宛先ごとの計算も要らない。
func (p *chatEventPublisher) broadcastRoomChanged(ctx context.Context, ev chatusecase.MessageSentEvent, roomGraphID string) {
	recipients, err := p.roomChangedRecipients(ctx, ev)
	if err != nil {
		// 宛先が引けなくてもメッセージ送信自体は成功しているので配信は続ける。
		// ただし黙って落とすと「一覧だけ古いまま」が障害として見えないのでログに残す。
		logChatDelivery(err, chatDeliveryRoomChanged).
			Int64("room_id", ev.Room.ID).
			Int64("message_id", ev.Message.ID).
			Msg("failed to resolve the recipients; skipping the room_changed broadcast")
		return
	}

	payload := map[string]any{
		"roomID":    roomGraphID,
		"messageID": encodeGraphID("message", ev.Message.ID),
		// 宛先から投稿者を除いてあるので、受け取った時点で必ず「自分以外の新着」。
		"hasNewMessage": true,
		"lastMessage":   messagePreview(ev.Message.Content, ev.HasMedia),
		"sentAt":        roomChangedSentAt(time.Now()),
	}
	for _, userID := range recipients {
		if userID == ev.ActorID {
			// 投稿者本人には送らない。自分の送信は送信元の画面が既に知っているし、
			// 「自分が投稿したせいで自分に新着が付く」のは誤りなので。
			continue
		}
		p.deps.SSEBroker.PublishToUser(userID, chatEventRoomChanged, payload)
	}
}

// roomChangedSentAt は room_changed に載せる「このイベントを組み立てた時刻」を作る。
// 値は Unix エポックからのナノ秒を10進で書いた文字列。
//
// # なぜ時刻を載せるのか（なぜ messageID で新旧を比べさせないのか）
//
// room_changed は順序を保証せずに配る（理由は broadcastRoomChanged と
// usecase/chat/async_events.go）。入れ替わって届いた古いイベントを受け取った側が
// 捨てられるよう、比較できる値を1つ載せる必要がある。
//
// その値に messageID は使えない。messageID は不透明ID（encodeGraphID）で、
// 新旧を知るには中の連番を解くしかない。解いた瞬間「クライアントはIDの中身を
// 解釈しない」という約束が壊れ、こちらが符号化を変えた日に、ビルドも通ったまま
// 順序判定だけが黙って退化する。時刻はそれ自体が意味を持つ値なので、IDの形が
// 変わっても影響を受けない。
//
// メッセージの created_at は使わない。秒解像度では同じ秒に並んだ2件を判定できず、
// 賑やかなルームでは判定したい場面ほど効かない。
//
// # なぜ数値ではなく文字列なのか
//
// UnixNano は現在およそ 1.7e18 で、JSON の数値のまま渡すと受け手の JS が
// Number（安全な整数は 9.0e15 まで）へ丸めて下位の桁を落とす。落ちるのは
// まさに同じミリ秒に並んだ2件を分ける桁なので、精度を捨てた時点で載せる意味が
// 薄れる。10進文字列なら桁を失わず、受け取った側は BigInt で正確に比べられる。
//
// 限界: 値は保存の順ではなく「イベントを組み立てた順」なので、非同期配信の起動が
// 前後すると保存の順と入れ替わりうる（保存の順まで揃えるには結局IDの中身を覗く
// しかない）。壁時計が巻き戻れば判定も鈍る。どちらも起きたところで一覧の
// プレビューが一瞬古いままになるだけで、未読数も本文も次の取り直しで必ず直る。
func roomChangedSentAt(now time.Time) string {
	return strconv.FormatInt(now.UnixNano(), 10)
}

// roomChangedRecipients は更新を知らせるべき利用者IDを、ルーム種別に応じて解決する。
//
// 授業ルームは room_users を使わない設計（誰でも閲覧でき匿名で表示する）なので、
// 履修者は時間割から引く。それ以外のルームは、送信の権限判定で既に room_users を
// 引いてあり（usecase/chat/access.go の writeAccess.MemberIDs）、イベントに載って
// 運ばれてくる。同じクエリを2度投げないためにそれをそのまま使う。
func (p *chatEventPublisher) roomChangedRecipients(ctx context.Context, ev chatusecase.MessageSentEvent) ([]int64, error) {
	if ev.Room.Type == model.RoomTypeCourse {
		return p.deps.CourseRegistrantIDs.Execute(ctx, ev.Room.ID)
	}
	return ev.MemberIDs, nil
}

// publishSentNotifications は送信に伴う通知（DM・引用返信・メンション）をまとめて出す。
// どれもベストエフォートで、失敗はログに残して先へ進む。
func (p *chatEventPublisher) publishSentNotifications(ctx context.Context, ev chatusecase.MessageSentEvent) {
	previewMessage := messagePreview(ev.Message.Content, ev.HasMedia)

	// DM ルームの場合、相手に通知を送る
	if ev.Room.Type == model.RoomTypeDM {
		targetType := notificationuc.TargetRoom
		for _, memberID := range ev.MemberIDs {
			if memberID == ev.ActorID {
				continue
			}
			if err := p.deps.NotificationPublisher.Publish(ctx, notificationuc.PublishParams{
				UserID:     memberID,
				Type:       notificationuc.TypeDM,
				ActorID:    &ev.ActorID,
				TargetType: &targetType,
				TargetID:   &ev.Room.ID,
				Message:    previewMessage,
			}); err != nil {
				logChatDelivery(err, chatDeliveryDMNotification).
					Int64("room_id", ev.Room.ID).
					Int64("message_id", ev.Message.ID).
					Int64("recipient_id", memberID).
					Msg("failed to publish dm notification")
			}
		}
	}

	// 引用返信の通知。DM は全メッセージで既に dm 通知が飛ぶので二重通知を避けて対象外。
	// notifiedByReply には返信通知を送った相手が入り、同じ人をメンションしていても
	// メンション通知が二重にならないようにする。
	var notifiedByReply *int64
	if ev.Message.ReplyToID != nil && ev.Room.Type != model.RoomTypeDM {
		notifiedByReply = p.publishMessageReplyNotification(ctx, ev.Room, ev.Message, ev.ActorID)
	}

	p.publishMentionNotifications(ctx, ev.Room, ev.Message, ev.ActorID, notifiedByReply)
}

// async は fn を deps.Async へ渡す。Async が無い配線ではその場で同期に実行する。
func (p *chatEventPublisher) async(ctx context.Context, name string, fn func(context.Context)) {
	if p.deps.Async == nil {
		fn(ctx)
		return
	}
	p.deps.Async.Go(ctx, name, fn)
}

func (p *chatEventPublisher) MessageUpdated(ctx context.Context, ev chatusecase.MessageUpdatedEvent) {
	// 購読配信は同期。message:added と同じ経路なので、非同期にすると編集が追加より
	// 先に届いて「まだ無いメッセージの編集」になりうる。
	roomGraphID := encodeGraphID("room", ev.Room.ID)
	p.deps.PubSub.Publish(roomGraphID+":message:updated", toGraphMessage(ev.Message))

	if len(ev.AddedMentions) > 0 {
		// 追加分だけを持つコピーを渡す（publishMentionNotifications は
		// msg.Mentions を宛先として読む）。イベントに載ってきた Message とは別物に
		// なるので、非同期でもクロージャが掴むのはこのコピーだけで済む。
		notified := *ev.Message
		notified.Mentions = ev.AddedMentions
		p.async(ctx, "message_updated", func(ctx context.Context) {
			p.publishMentionNotifications(ctx, ev.Room, &notified, ev.ActorID, nil)
		})
	}
}

// MessageDeleted は全て同期。購読配信は順序が要り（削除が追加を追い越すと
// 消えないメッセージが残る）、監査ログと合わせても DB アクセスは無く安価なので、
// リクエストの外へ出す理由が無い。
func (p *chatEventPublisher) MessageDeleted(ctx context.Context, ev chatusecase.MessageDeletedEvent) {
	audit.LogMessageDeleted(ctx, ev.RoomID, ev.MessageID)
	roomGraphID := encodeGraphID("room", ev.RoomID)
	p.deps.PubSub.Publish(roomGraphID+":message:deleted", &gqlmodel.Message{ID: encodeGraphID("message", ev.MessageID)})
}

func (p *chatEventPublisher) RoomMarkedAsRead(ctx context.Context, ev chatusecase.RoomMarkedAsReadEvent) {
	roomGraphID := encodeGraphID("room", ev.Room.ID)

	// 既読の配信は相手側の既読表示のためのもの。授業内チャットは匿名なので、
	// 誰が読んだか(実ユーザーID)をルームの購読者へ配信しない。
	//
	// 購読配信なので同期（message:* と同じ経路を使う。「送信 → 既読」が逆転すると
	// 既読表示が巻き戻って見える）。
	if ev.Room.Type != model.RoomTypeCourse {
		nowStr := time.Now().Format(timeFormat)
		p.deps.PubSub.Publish(roomGraphID+":read_status", &gqlmodel.RoomReadStatusUpdate{
			UserID:     encodeGraphID("user", ev.ActorID),
			LastReadAt: nowStr,
		})
	}

	// 通知の既読化（DM）と、既読を打った本人への room_changed はリクエストの外へ。
	// どちらも DB アクセスを伴い、既読という操作の応答を待たせる理由が無い。
	p.async(ctx, "room_marked_as_read", func(ctx context.Context) {
		if ev.Room.Type == model.RoomTypeDM {
			p.syncDMNotificationsOnRead(ctx, ev)
		}

		// 既読を打った本人へ「このルームが更新された」を送る。
		//
		// 以前はここで unreadCount: 0 を送っていたが、既読位置をクライアントが
		// 指定できる以上「画面に出した最後まで」しか読んでいないこともあり、0 は
		// 嘘になりうる数字だった。数を送るのをやめれば嘘をつく余地が無くなる。
		// 受け取った側は自分の未読数を取り直せばよい。
		//
		// 既読を打った端末自身は自分で一覧を取り直すので、これが効くのは同じ利用者の
		// 別のタブ・別の端末。hasNewMessage は false（新着ではなく既読による変化で、
		// 一覧のプレビューを書き換えてはいけない）。messageID はイベントが持たないので
		// 載せない（載せる値が無いだけで、プレビューを触らない以上、新旧の判定も要らない）。
		//
		// sentAt も載せない。あれは「古いイベントでプレビューが巻き戻るのを防ぐ」ための
		// 値で、プレビューを触らないこのイベントには使い道が無い（受け取った側は
		// hasNewMessage: false を見た時点で未読数を取り直すだけ）。載せれば新着の
		// sentAt と混ざり、既読の時刻で新着の判定が進んでしまう＝直後に届いた本物の
		// 新着を捨てかねないので、載せないほうが安全でもある。
		p.deps.SSEBroker.PublishToUser(ev.ActorID, chatEventRoomChanged, map[string]any{
			"roomID":        roomGraphID,
			"hasNewMessage": false,
		})
	})
}

// syncDMNotificationsOnRead は DM を既読にしたとき、相手からの DM 通知も既読にして、
// 通知の状態が変わったことを本人へ知らせる。
//
// 以前はここで未読通知数を COUNT して数字ごと配っていた。やめたのは room_changed と
// 同じ理由で、派生値をサーバが配らないため（sse.EventNotificationsChanged のコメント参照）。
// 受け取った側（＝既読を打った本人の他のタブ・他の端末）は自分で数を取り直す。
// 既読を打った端末自身も、mutation の応答を見て自分で数を直せる。
func (p *chatEventPublisher) syncDMNotificationsOnRead(ctx context.Context, ev chatusecase.RoomMarkedAsReadEvent) {
	for _, memberID := range ev.MemberIDs {
		if memberID == ev.ActorID {
			continue
		}
		if err := p.deps.MarkNotificationsAsReadByActor.Execute(ctx, ev.ActorID, string(notificationuc.TypeDM), memberID); err != nil {
			logChatDelivery(err, chatDeliveryDMReadSync).
				Int64("room_id", ev.Room.ID).
				Int64("actor_id", ev.ActorID).
				Int64("partner_id", memberID).
				Msg("failed to mark dm notifications as read")
		}
	}
	p.deps.SSEBroker.PublishNotificationsChangedToUser(ev.ActorID)
}

// publishMessageReplyNotification notifies the author of the message that `reply`
// quotes. 自分自身への返信、および返信先が見つからない（削除済み）場合は何もしない。
//
// 授業内チャットは匿名なので、通知文言にはルーム内の匿名ラベルを入れ、actor_id は
// あえて保存しない。actor_id を残すと myNotifications(actorID:) や
// markAllNotificationsAsReadByActor など actor で絞り込むAPIから
// 「匿名NNN = そのユーザー」を突き合わせられてしまい、匿名性が崩れるため。
//
// 遷移先の組み立てにはルームIDと種別が要るが notifications 行は targetType/targetID の
// 1組しか持てないため、SSE には Extra で roomID/roomType を添える（GraphQL 側は
// Notification.targetMessage から辿れる）。
//
// 通知の失敗はメッセージ送信の成否に影響させない（ログのみ）。
//
// 戻り値は通知を送った相手のユーザーID（送らなかったときは nil）。
// 同じ相手をメンションしていたときにメンション通知を二重に送らないために使う。
func (p *chatEventPublisher) publishMessageReplyNotification(ctx context.Context, room *model.Room, reply *model.Message, actorID int64) *int64 {
	if reply.ReplyToID == nil {
		return nil
	}

	parent, err := p.deps.GetMessage.Execute(ctx, *reply.ReplyToID)
	if err != nil {
		// 返信元が引けないときは通知を諦める（挙動は従来どおり）。返信先が削除済みで
		// nil が返るのは正常系なので、error のときだけ残す。
		logChatDelivery(err, chatDeliveryReplyParentLookup).
			Int64("room_id", room.ID).
			Int64("message_id", reply.ID).
			Int64("reply_to_id", *reply.ReplyToID).
			Msg("failed to load the quoted message; skipping the reply notification")
		return nil
	}
	if parent == nil {
		return nil
	}
	if parent.UserID == actorID {
		return nil
	}

	// 「どこで返信されたか」は文言に入る（文言の組み立ては usecase/notification の担当）。
	// 通知一覧・通知詳細・SSE のトーストはどれも message をそのまま出すので、
	// 文言に入れておけば3箇所ぶんの組み立てが要らない。
	message := notificationuc.MessageRepliedInRoom(room.Name)
	notificationActorID := &actorID
	if room.Type == model.RoomTypeCourse {
		// 匿名IDは投稿時に確定済みなので、ここは採番せず読むだけ。行が引けなくても
		// 実名を出すわけにはいかないので、番号なしの「匿名」で通知する。
		label := anonymousPlaceholderLabel
		identity, err := p.deps.GetAnonymousIdentity.Execute(ctx, room.ID, actorID)
		if err != nil {
			logChatDelivery(err, chatDeliveryAnonymousLabel).
				Int64("room_id", room.ID).
				Int64("message_id", reply.ID).
				Msg("failed to resolve anonymous identity for reply notification")
		} else if identity != nil {
			label = identity.Label
		}
		message = notificationuc.MessageAnonymousRepliedInRoom(room.Name, label)
		notificationActorID = nil
	}

	targetType := notificationuc.TargetMessage
	if err := p.deps.NotificationPublisher.Publish(ctx, notificationuc.PublishParams{
		UserID:     parent.UserID,
		Type:       notificationuc.TypeMessageReply,
		ActorID:    notificationActorID,
		TargetType: &targetType,
		TargetID:   &reply.ID,
		Message:    message,
		Extra: map[string]any{
			"roomID":   encodeGraphID("room", room.ID),
			"roomType": room.Type,
		},
	}); err != nil {
		logChatDelivery(err, chatDeliveryReplyNotification).
			Int64("room_id", room.ID).
			Int64("message_id", reply.ID).
			Int64("recipient_id", parent.UserID).
			Msg("failed to publish message reply notification")
		return nil
	}
	return &parent.UserID
}

// publishMentionNotifications はコミュニティチャットでメンションされた各ユーザーへ通知する。
//
// skipUserID には引用返信の通知を既に送った相手を渡す。返信と同時にその相手を
// メンションしても通知が2通にならないようにするため。
// 遷移先の組み立てにはルームIDと種別が要るが notifications 行は targetType/targetID の
// 1組しか持てないため、SSE には Extra で roomID/roomType を添える
// （publishMessageReplyNotification と同じ扱い）。
// 通知の失敗はメッセージ送信の成否に影響させない（ログのみ）。
func (p *chatEventPublisher) publishMentionNotifications(ctx context.Context, room *model.Room, msg *model.Message, actorID int64, skipUserID *int64) {
	if len(msg.Mentions) == 0 {
		return
	}

	// 「どこでメンションされたか」は文言に入る（返信通知と同じ理由。
	// publishMessageReplyNotification のコメントを参照）。
	message := notificationuc.MessageMentionedInRoom(room.Name)

	targetType := notificationuc.TargetMessage
	params := make([]notificationuc.PublishParams, 0, len(msg.Mentions))
	for _, m := range msg.Mentions {
		if skipUserID != nil && m.UserID == *skipUserID {
			continue
		}
		params = append(params, notificationuc.PublishParams{
			UserID:     m.UserID,
			Type:       notificationuc.TypeMessageMention,
			ActorID:    &actorID,
			TargetType: &targetType,
			TargetID:   &msg.ID,
			Message:    message,
			Extra: map[string]any{
				"roomID":   encodeGraphID("room", room.ID),
				"roomType": room.Type,
			},
		})
	}
	if len(params) == 0 {
		return
	}

	if err := p.deps.NotificationPublisher.PublishBatch(ctx, params); err != nil {
		logChatDelivery(err, chatDeliveryMentionNotify).
			Int64("room_id", room.ID).
			Int64("message_id", msg.ID).
			Int("recipients", len(params)).
			Msg("failed to publish mention notifications")
	}
}

// messagePreview は一覧画面のプレビュー・通知文言に使う要約テキストを作る。
// 本文が空でも添付だけの送信はありうるので、そのときは代替文言を返す。
func messagePreview(content string, hasMedia bool) string {
	if content == "" {
		if hasMedia {
			return "[画像/ファイルを送信しました]"
		}
		return "新しいメッセージが届きました"
	}

	const maxPreviewRunes = 50
	if runes := []rune(content); utf8.RuneCountInString(content) > maxPreviewRunes {
		return string(runes[:maxPreviewRunes]) + "…"
	}
	return content
}
