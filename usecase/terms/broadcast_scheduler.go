package terms

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// termsUpdatedEvent は「利用規約が新しい版に切り替わった」ことをクライアントへ知らせる
// SSE イベント名。クライアントはこれを受けて同意状態を取り直す。
const termsUpdatedEvent = "terms_updated"

// TermsBroadcaster は terms_updated を全接続へ配信できるもの。internal/sse.Broker が
// これを満たす。usecase 層が internal/sse に直接依存しないよう、必要な1メソッドだけを
// ポートとして切ってある。
type TermsBroadcaster interface {
	Broadcast(eventType string, data map[string]any)
}

// BroadcastScheduler は「effectiveDate になったら terms_updated を配信する」タイマーを
// 一手に引き受ける。以前は起動時（cmd/server）と作成時（GraphQL リゾルバ）に同じ処理の
// 別実装があり、片方だけ直すと挙動がずれる状態だった。入口は次の2つだけ:
//
//   - SchedulePending: 起動時に、未来日付で登録済みの全バージョンぶんのタイマーを張る
//   - Schedule:        作成時に、いま作られた1バージョンぶんのタイマーを張る
//
// 制約（この実装では解決していない）:
// タイマーはプロセスのメモリ上にあるだけなので、アプリを複数インスタンスで動かすと
// 各インスタンスが同じ時刻に同じ terms_updated を配信する（重複発火）。また作成時は
// 「作成を受けたインスタンス」だけがタイマーを持つため、そのインスタンスが
// effectiveDate 前に落ちると、次に誰かが再起動するまで配信されない。
// 本当に1回だけ確実に配信するには分散ロックかリーダー選出が要る。ここで直したのは
// 「同じ処理の実装が2箇所にあった」ことだけで、多重配信の解消は別途対応が必要。
type BroadcastScheduler struct {
	termsRepo   repository.TermsRepository
	broadcaster TermsBroadcaster
}

func NewBroadcastScheduler(termsRepo repository.TermsRepository, broadcaster TermsBroadcaster) *BroadcastScheduler {
	return &BroadcastScheduler{termsRepo: termsRepo, broadcaster: broadcaster}
}

// Schedule は effectiveDate が既に到来していれば即座に配信し、未来ならその時刻に
// 発火する一度きりのタイマーを張る。
func (s *BroadcastScheduler) Schedule(version string, effectiveDate time.Time) {
	if s == nil || s.broadcaster == nil {
		return
	}
	delay := time.Until(effectiveDate)
	if delay <= 0 {
		s.broadcast(version)
		return
	}
	time.AfterFunc(delay, func() { s.broadcast(version) })
	logger.Log.Info().Str("version", version).Dur("delay", delay).Msg("scheduled terms broadcast timer set")
}

// SchedulePending は未来日付で登録済みの全バージョンにタイマーを張る。起動時に呼ぶ。
func (s *BroadcastScheduler) SchedulePending(ctx context.Context) {
	if s == nil || s.broadcaster == nil {
		return
	}
	pending, err := s.termsRepo.FindFuture(ctx)
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to fetch pending future terms on startup")
		return
	}
	for _, t := range pending {
		s.Schedule(t.Version, t.EffectiveDate)
	}
}

func (s *BroadcastScheduler) broadcast(version string) {
	s.broadcaster.Broadcast(termsUpdatedEvent, map[string]any{"version": version})
	logger.Log.Info().Str("version", version).Msg("terms now effective, SSE broadcast sent")
}
