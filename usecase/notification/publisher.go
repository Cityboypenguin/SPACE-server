package notification

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type NotificationType string

const (
	TypeFavorite      NotificationType = "favorite"
	TypeReply         NotificationType = "reply"
	TypeDM            NotificationType = "dm"
	TypeCommunityKick NotificationType = "community_kick"
	TypeCommunityRole NotificationType = "community_role"
	TypeAnnouncement  NotificationType = "announcement"
	TypeFollow        NotificationType = "follow"
	// TypeMessageReply はチャット内の引用返信。投稿への返信 (TypeReply) とは
	// 遷移先が異なる（ルームを開いて該当メッセージへジャンプする）ため別タイプにしている。
	TypeMessageReply NotificationType = "message_reply"
	// TypeMention は投稿本文での @accountID メンション。遷移先は投稿。
	TypeMention NotificationType = "mention"
	// TypeMessageMention はコミュニティチャットでの @表示名 メンション。
	// 返信と同じく遷移先がルーム内の該当メッセージなので、投稿のメンションとは別タイプにしている。
	TypeMessageMention NotificationType = "message_mention"
)

type TargetType string

const (
	TargetPost         TargetType = "post"
	TargetRoom         TargetType = "room"
	TargetCommunity    TargetType = "community"
	TargetAnnouncement TargetType = "announcement"
	TargetMessage      TargetType = "message"
)

type PublishParams struct {
	UserID     int64
	Type       NotificationType
	ActorID    *int64
	TargetType *TargetType
	TargetID   *int64
	Message    string
	// Extra はリアルタイム配信のペイロードにだけ載せる追加フィールド。
	// DB の notifications 行は TargetType/TargetID の1組しか持てないため、
	// 遷移先の組み立てに足りない情報（返信通知のルームIDなど）をここで補う。
	Extra map[string]any
}

// UserEventDelivery はリアルタイムイベントをユーザーへ配信するポート。
// usecase 層はこのインターフェースに依存し、具体的な配信手段（SSE 等）を知らない。
type UserEventDelivery interface {
	PublishToUser(userID int64, eventType string, data map[string]any)

	// ConnectedUserIDs はいま接続している利用者のIDを返す。
	//
	// 全員宛の配信（お知らせ）で「誰に送るか」を決めるために要る。配信手段が
	// 「今つながっている相手」を知っていること自体は SSE でも WebSocket でも
	// 変わらないので、ポートに置いても実装先を縛らない。
	ConnectedUserIDs() []int64
}

// NotificationPublisher は通知を DB に保存し、接続中のユーザーへリアルタイム配信する。
type NotificationPublisher interface {
	Publish(ctx context.Context, params PublishParams) error
	PublishBatch(ctx context.Context, params []PublishParams) error

	// PublishToAllActiveUsers は同じ通知を全アクティブ利用者へ配る（お知らせ）。
	//
	// PublishBatch との違いは宛先の決まり方。PublishBatch は呼び出し側が宛先を
	// 並べて渡す（返信・メンションのように相手が数人）のに対し、こちらは宛先を
	// アプリが列挙しない。保存は DB 内の INSERT ... SELECT で閉じ、配信は
	// 接続中の利用者にだけ行う。
	PublishToAllActiveUsers(ctx context.Context, params BroadcastParams) error
}

// BroadcastParams は PublishToAllActiveUsers の指定。PublishParams から UserID を
// 抜いた形（宛先はアプリではなく DB と Broker が決める）。
type BroadcastParams struct {
	Type       NotificationType
	TargetType TargetType
	TargetID   int64
	Message    string
}

type notificationPublisher struct {
	repo     repository.NotificationRepository
	delivery UserEventDelivery
}

func NewNotificationPublisher(repo repository.NotificationRepository, delivery UserEventDelivery) NotificationPublisher {
	return &notificationPublisher{repo: repo, delivery: delivery}
}

// deliveryData builds the realtime payload for one notification, applying the
// per-notification masking (HideActor) and extra fields from params.
func deliveryData(n *model.Notification, params PublishParams) map[string]any {
	data := map[string]any{
		"id":        opaqueid.Encode("notification", n.ID),
		"type":      n.Type,
		"message":   n.Message,
		"isRead":    false,
		"createdAt": n.CreatedAt.Format(time.RFC3339),
	}
	if n.ActorID != nil {
		data["actorID"] = opaqueid.Encode("user", *n.ActorID)
	}
	if n.TargetType != nil {
		data["targetType"] = *n.TargetType
	}
	if n.TargetID != nil {
		data["targetID"] = opaqueid.Encode(*n.TargetType, *n.TargetID)
	}
	for k, v := range params.Extra {
		data[k] = v
	}
	return data
}

func (p *notificationPublisher) PublishBatch(ctx context.Context, params []PublishParams) error {
	if len(params) == 0 {
		return nil
	}
	ns := make([]*model.Notification, 0, len(params))
	for _, param := range params {
		var targetTypeStr *string
		if param.TargetType != nil {
			t := string(*param.TargetType)
			targetTypeStr = &t
		}
		ns = append(ns, model.NewNotification(model.CreateNotificationParam{
			UserID:     param.UserID,
			Type:       string(param.Type),
			ActorID:    param.ActorID,
			TargetType: targetTypeStr,
			TargetID:   param.TargetID,
			Message:    param.Message,
		}))
	}

	if err := p.repo.SaveBatch(ctx, ns); err != nil {
		return err
	}

	for i, n := range ns {
		p.delivery.PublishToUser(params[i].UserID, "notification", deliveryData(n, params[i]))
	}
	return nil
}

func (p *notificationPublisher) Publish(ctx context.Context, params PublishParams) error {
	var targetTypeStr *string
	if params.TargetType != nil {
		t := string(*params.TargetType)
		targetTypeStr = &t
	}

	n := model.NewNotification(model.CreateNotificationParam{
		UserID:     params.UserID,
		Type:       string(params.Type),
		ActorID:    params.ActorID,
		TargetType: targetTypeStr,
		TargetID:   params.TargetID,
		Message:    params.Message,
	})

	if err := p.repo.Save(ctx, n); err != nil {
		return err
	}

	p.delivery.PublishToUser(params.UserID, "notification", deliveryData(n, params))
	return nil
}

// PublishToAllActiveUsers は全アクティブ利用者への通知を保存し、接続中の利用者にだけ配信する。
//
// 以前は (1) 全アクティブ利用者IDを SELECT でアプリへ読み込み、(2) 人数ぶんの
// PublishParams と通知モデルを組み、(3) 人数ぶんの VALUES を並べた INSERT を撃ち、
// (4) 全員へ SSE を送っていた。利用者数に比例してメモリ・SQL 文長・イベント数が
// 増え、しかも (4) は接続していない人の履歴（Broker の history）にも積まれていた
// （履歴の用途は再接続時のリプレイだけなので、繋いでいない人のぶんは TTL の間
// メモリを占めるだけ）。
//
// ここでは3つに分ける:
//
//  1. 保存は INSERT ... SELECT で DB 内に閉じる（行の中身をアプリへ運ばない）
//  2. 宛先は Broker が知っている「いま接続している人」だけに絞る
//  3. 配信ペイロードに要る通知IDは、その接続中の人ぶんだけ引き直す
//
// 配信するイベントは今までと同じ "notification"（中身も同じ）。事実だけを送る
// notifications_changed に寄せなかったのは、お知らせはトーストを出す通知であり、
// 事実だけに変えると接続中の利用者からトーストが消える＝外から見える挙動が
// 変わってしまうため。notifications_changed の考え方（サーバは配れるものだけを
// 配り、配れないものを無理に配らない）は「接続していない人へは送らない」という
// 形でここに効いている。切断中に作られた通知は、再接続時にクライアントが
// 通知一覧と未読数を取り直すので取りこぼしにはならない。
func (p *notificationPublisher) PublishToAllActiveUsers(ctx context.Context, params BroadcastParams) error {
	targetType := string(params.TargetType)
	created, err := p.repo.SaveForAllActiveUsers(ctx, repository.BroadcastNotificationParam{
		Type:       string(params.Type),
		TargetType: &targetType,
		TargetID:   &params.TargetID,
		Message:    params.Message,
		CreatedAt:  time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	logger.Log.Info().
		Str("component", "notification_broadcast").
		Str("notification_type", string(params.Type)).
		Str("target_type", targetType).
		Int64("target_id", params.TargetID).
		Int64("created_rows", created).
		Msg("stored a broadcast notification for every active user")

	connected := p.delivery.ConnectedUserIDs()
	if len(connected) == 0 {
		return nil
	}

	// 通知IDは配信ペイロード（クライアントの遷移先）に要るので引き直す。
	// 引くのは接続中の人ぶんだけなので、全件を運ぶことにはならない。
	ns, err := p.repo.ListByTargetForUsers(ctx, targetType, params.TargetID, connected)
	if err != nil {
		// 配信できなくても保存は済んでいる（再接続時に取り直される）ので、
		// お知らせ作成そのものは失敗させない。黙って落とすと「一部の人にだけ
		// 届かない」が誰にも気づかれないので、必ずログには残す。
		logger.Log.Error().Err(err).
			Str("component", "notification_broadcast").
			Int64("target_id", params.TargetID).
			Int("connected_users", len(connected)).
			Msg("failed to look up broadcast notifications for delivery")
		return nil
	}

	for _, n := range ns {
		p.delivery.PublishToUser(n.UserID, "notification", deliveryData(n, PublishParams{}))
	}
	return nil
}
