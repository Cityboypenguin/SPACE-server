package sse

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

// Envelope は1件の配信。UserID 0 は「接続中の全員へ」。
type Envelope struct {
	UserID int64 `json:"userID"`
	Event  Event `json:"event"`
}

// Fanout はイベントを全ての台へ配る口。
//
// SSE の接続はどこか1台に貼り付くが、イベントを起こす操作（誰かが通知を作る、
// 誰かがメッセージを送る）は別の台に当たりうる。配信が自分の台のクライアントに
// しか届かないと、通知を作った台に繋がっていない利用者にはベルが光らない。
// 「届かない」だけでエラーにはならないので、1台で動かしている間は誰も気づけない。
type Fanout interface {
	Publish(ctx context.Context, env Envelope) error
}

// localFanout は自分の台のクライアントへ返すだけの配線。台が1つの構成向け。
//
// 台が1つでも必ず Fanout を通すのは、Publish 側に「台が複数なら…」という分岐を
// 作らないため。分岐を作ると、片方の経路だけ直し忘れる。
type localFanout struct{ b *Broker }

var _ Fanout = localFanout{}

func (f localFanout) Publish(_ context.Context, env Envelope) error {
	f.b.DeliverLocal(env)
	return nil
}

// --- リプレイ時のログ ------------------------------------------------------
//
// 置き場（Store）の実装ごとに同じことを書かないよう、ここにまとめてある。

func logReplayNoHistory(userID int64, lastEventID int) {
	logger.Log.Debug().
		Int64("userID", userID).
		Int("lastEventID", lastEventID).
		Msg("SSE reconnect with no history for this user: nothing to replay")
}

func logReplayAfterRestart(userID int64, lastEventID, currentNextID int) {
	logger.Log.Warn().
		Int64("userID", userID).
		Int("lastEventID", lastEventID).
		Int("currentNextID", currentNextID).
		Msg("SSE reconnect after server restart: Last-Event-ID exceeds this user's counter, skipping replay")
}

// SelectMissed は履歴から lastEventID より新しいものを選ぶ。
//
// 公開しているのは、置き場の実装（infra/redis）が同じ選び方を使うため。
// ここを写経すると、片方だけ「溢れたときの警告」が消えるといったずれ方をする。
//
// 履歴から溢れて消えたぶんがある場合は警告を残す。採番がユーザー単位なので、
// ここが立つのは本当に溢れた時だけ（以前は他人宛に消費された欠番でも立っていた）。
func SelectMissed(userID int64, lastEventID int, hist []Event) []Event {
	if len(hist) > 0 && hist[0].ID > lastEventID+1 {
		logger.Log.Warn().
			Int64("userID", userID).
			Int("lastEventID", lastEventID).
			Int("oldestHistoryID", hist[0].ID).
			Msg("SSE replay gap: some events evicted from history; client may have missed notifications")
	}
	var missed []Event
	for _, ev := range hist {
		if ev.ID > lastEventID {
			missed = append(missed, ev)
		}
	}
	return missed
}
