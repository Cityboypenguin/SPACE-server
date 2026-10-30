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

// BroadcastOnce は「この版の配信をこの台が引き受けてよいか」を、台をまたいで
// 1回だけ true にする門番。
//
// タイマーはプロセスのメモリ上にあるので、台を増やすと全ての台が同じ時刻に
// 目を覚ます。誰が配るかをここで決めないと、台数ぶん同じ terms_updated が飛ぶ。
// 印は消さない（version ごとに1個、年に数回しか増えない）。消してしまうと
// 次の起動でまた配ってしまい、1回だけという約束が保てない。
type BroadcastOnce interface {
	// Claim は version の配信権を取りに行く。取れたら true。
	// 既に誰か（別の台、または再起動前の自分）が配った後なら false。
	Claim(ctx context.Context, version string) (bool, error)
}

// BroadcastScheduler は「effectiveDate になったら terms_updated を配信する」タイマーを
// 一手に引き受ける。以前は起動時（cmd/server）と作成時（GraphQL リゾルバ）に同じ処理の
// 別実装があり、片方だけ直すと挙動がずれる状態だった。入口は次の2つだけ:
//
//   - SchedulePending: 起動時に、取りこぼしを拾い、未来日付のぶんのタイマーを張る
//   - Schedule:        作成時に、いま作られた1バージョンぶんのタイマーを張る
//
// 台を複数立てる構成では BroadcastOnce を渡すこと。渡さないと、台数ぶん重複して
// 配信される。渡した場合は次の2つがどちらも1回に収まる:
//
//   - 同じ時刻に複数の台のタイマーが発火したとき（重複発火）
//   - 発効時刻をアプリの停止中に跨いだとき（取りこぼし。起動時に拾う）
type BroadcastScheduler struct {
	termsRepo   repository.TermsRepository
	broadcaster TermsBroadcaster
	// once が nil なら門番なしで配る。台が1つだけの構成とテスト用。
	once BroadcastOnce
}

func NewBroadcastScheduler(termsRepo repository.TermsRepository, broadcaster TermsBroadcaster, once BroadcastOnce) *BroadcastScheduler {
	return &BroadcastScheduler{termsRepo: termsRepo, broadcaster: broadcaster, once: once}
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

// SchedulePending は起動時に呼ぶ。未来日付で登録済みの全バージョンにタイマーを張り、
// その前に「発効済みなのにまだ配信されていない版」を拾う。
func (s *BroadcastScheduler) SchedulePending(ctx context.Context) {
	if s == nil || s.broadcaster == nil {
		return
	}

	s.catchUpCurrent(ctx)

	pending, err := s.termsRepo.FindFuture(ctx)
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to fetch pending future terms on startup")
		return
	}
	for _, t := range pending {
		s.Schedule(t.Version, t.EffectiveDate)
	}
}

// catchUpCurrent は「発効時刻を過ぎているのに配信されていない版」を起動時に配る。
//
// FindFuture が返すのは未来の版だけなので、発効時刻をアプリの停止中に跨ぐと
// （デプロイでの入れ替えを含む）その版の terms_updated は誰も配らないまま終わる。
// 起動のたびに現行版を配りに行き、配信済みかどうかの判断は門番に任せる。
//
// 門番が居ないときは何もしない。印が無いと「配信済みかどうか」を知る手立てが
// 無く、起動のたびに配ってしまうため。
func (s *BroadcastScheduler) catchUpCurrent(ctx context.Context) {
	if s.once == nil {
		return
	}
	current, err := s.termsRepo.FindCurrent(ctx)
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to fetch the current terms on startup")
		return
	}
	if current == nil {
		return
	}
	s.broadcast(current.Version)
}

func (s *BroadcastScheduler) broadcast(version string) {
	if s.once != nil {
		claimed, err := s.once.Claim(context.Background(), version)
		switch {
		case err != nil:
			// 印を取れなかったのか、Redis が応答しないのかを区別できない。配るほうへ倒す。
			// 重複して届いてもクライアントは同意状態を取り直すだけだが、届かないと
			// 新しい規約に気づけないまま使い続けることになる。
			logger.Log.Error().Err(err).Str("version", version).
				Msg("failed to claim the terms broadcast; broadcasting anyway")
		case !claimed:
			logger.Log.Info().Str("version", version).
				Msg("terms broadcast already claimed elsewhere; skipping")
			return
		}
	}
	s.broadcaster.Broadcast(termsUpdatedEvent, map[string]any{"version": version})
	logger.Log.Info().Str("version", version).Msg("terms now effective, SSE broadcast sent")
}
