package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// BroadcastNotificationParam は「同じ内容の通知を全アクティブ利用者ぶん作る」指定。
//
// PublishParams と違って UserID を持たないのが要点。宛先はアプリではなく DB が
// 決める（INSERT ... SELECT の SELECT 側）ので、利用者IDをアプリへ持ってくる必要が
// そもそも無い。
type BroadcastNotificationParam struct {
	Type       string
	TargetType *string
	TargetID   *int64
	Message    string
	CreatedAt  int64
}

type NotificationRepository interface {
	Save(ctx context.Context, n *model.Notification) error
	SaveBatch(ctx context.Context, ns []*model.Notification) error

	// SaveForAllActiveUsers は全アクティブ利用者ぶんの通知行を DB の中だけで作り、
	// 作った行数を返す。
	//
	// 以前はお知らせ作成が「全アクティブ利用者IDを SELECT でアプリへ読み込む →
	// 人数ぶんの構造体を組む → 人数ぶんの VALUES を並べた1本の INSERT」をしていた。
	// 利用者が増えるほどメモリも SQL 文の長さも人数に比例して膨らみ、
	// max_allowed_packet やプレースホルダ上限（65535）に当たる。行を作るのに
	// 中身をアプリへ運ぶ理由は無いので、INSERT ... SELECT で DB 内に閉じる。
	SaveForAllActiveUsers(ctx context.Context, p BroadcastNotificationParam) (int64, error)

	// ListByTargetForUsers は同じ通知先(target)の行のうち、指定した利用者ぶんだけを引く。
	//
	// SaveForAllActiveUsers は行をアプリへ返さない（返させると全件を運ぶことになり
	// 元の木阿弥）。一方でリアルタイム配信のペイロードには通知IDが要る。宛先は
	// 接続中の利用者だけなので、その人数ぶんだけをここで引き直す。
	ListByTargetForUsers(ctx context.Context, targetType string, targetID int64, userIDs []int64) ([]*model.Notification, error)

	ListByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.Notification, int, error)
	GetByID(ctx context.Context, id int64, userID int64) (*model.Notification, error)
	ListGroupedByUserID(ctx context.Context, userID int64, q PageQuery) ([]*model.NotificationGroup, int, error)
	ListByActor(ctx context.Context, userID int64, notifType string, actorID int64, q PageQuery) ([]*model.Notification, int, error)
	MarkAsRead(ctx context.Context, id int64, userID int64) error
	MarkAllAsRead(ctx context.Context, userID int64) error
	MarkAllAsReadByActor(ctx context.Context, userID int64, notifType string, actorID int64) error
	CountUnread(ctx context.Context, userID int64) (int, error)
	DeleteReadByUserID(ctx context.Context, userID int64) error
	DeleteReadByActor(ctx context.Context, userID int64, notifType string, actorID int64) error
	DeleteByIDs(ctx context.Context, ids []int64, userID int64) error
}
