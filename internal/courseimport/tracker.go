// Package courseimport tracks the status of the (long-running, sequential,
// curl-shelling) course scrape-and-import job so it can be triggered from a
// GraphQL mutation without blocking the request/response cycle. Status is kept
// in process memory only (lost on server restart) — this is an operator tool
// run occasionally by admins, not something that needs durable job history.
package courseimport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

// progressStallTimeout aborts a RUNNING import if it goes this long without a
// single progress report. Every unit of real work the scraper does (one listing
// page, one disambiguation detail-page fetch) is bounded by curl's own retry
// budget - curlMaxAttempts attempts at up to curlMaxTime seconds each, plus
// backoff between them, currently ~4 minutes worst case (see
// infra/scraper/su_syllabus.go) - so a healthy run, even one grinding through
// retries on a single flaky page, reports progress well within this window. It
// exists purely as a backstop against a genuine hang (e.g. a deadlock, or a curl
// child process that somehow ignores its own --max-time) leaving the tracker
// stuck RUNNING forever with no way to start a new import. Keep this comfortably
// above that worst-case retry budget, not just above the common case, or a
// request stalled on its last legitimate retry attempt gets killed right before
// it might have succeeded.
const progressStallTimeout = 10 * time.Minute

type State string

const (
	StateIdle      State = "IDLE"
	StateRunning   State = "RUNNING"
	StateSucceeded State = "SUCCEEDED"
	StateFailed    State = "FAILED"
)

type Status struct {
	State        State
	Year         int
	Imported     int
	Skipped      int
	ErrorMessage string
	StartedAt    *time.Time
	FinishedAt   *time.Time
	// Processed and Total describe progress of a RUNNING scrape (rows fetched so
	// far / total rows reported by the source site). Both are 0 until the first
	// progress report arrives, and are left at their last value once the run
	// finishes (State moves to SUCCEEDED/FAILED before the caller can react to it).
	Processed int
	Total     int
}

type Tracker struct {
	mu sync.Mutex
	// status はこの台が走らせている取り込みの状態。共有の記録は store 側にあり、
	// ここはその写し（進捗の組み立てと、store が引けなかったときの控え）。
	status Status
	// runToken は「今このトラッカーの状態を書いてよい実行」の合言葉。
	//
	// 印が寿命で解けた後に同じ台で次の実行が始まると、古い実行がまだ生きている
	// まま2本が並ぶことがある。共有の記録は token で弾けるが、この台の写し
	// （status）も古い実行に書き換えられると、管理画面の表示が実行と食い違う。
	runToken string
	store    Store
	onChange func(Status)
	ctx      context.Context
	cancel   context.CancelFunc
	stopping bool
	jobs     sync.WaitGroup
}

// NewTracker builds a Tracker that calls onChange (if non-nil) with a snapshot of
// the status every time it changes - Start, SetProgress, and completion - so a
// caller can push updates (e.g. over a GraphQL subscription) instead of relying on
// callers to poll Get.
//
// store は状態の置き場（Store のコメント参照）。nil ならプロセス内に置く
// （台が1つの構成向け。台を増やすなら Redis の実装を渡すこと）。
func NewTracker(store Store, onChange func(Status)) *Tracker {
	if store == nil {
		store = NewMemoryStore()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Tracker{status: Status{State: StateIdle}, store: store, onChange: onChange, ctx: ctx, cancel: cancel}
}

func (t *Tracker) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	t.stopping = true
	t.cancel()
	t.mu.Unlock()
	done := make(chan struct{})
	go func() {
		t.jobs.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Get は現在の状態を返す。
//
// 共有の記録を見るので、取り込みを始めた台とは別の台へ聞いても同じ答えが返る。
// 引けなかったときだけ自分の台の写しを返す（管理画面が真っ白になるより、
// 少し古いかもしれない値を出す方がまし）。
func (t *Tracker) Get() Status {
	status, err := t.store.Load(t.ctx)
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to load the course import status; falling back to this instance's copy")
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.status
	}
	return status
}

// notify reads the current status under the lock and then calls onChange with that
// snapshot after releasing it, so onChange (which may be slow, e.g. fanning out to
// subscribers) never runs while holding the mutex.
func (t *Tracker) notify() {
	t.mu.Lock()
	snapshot := t.status
	t.mu.Unlock()
	if t.onChange != nil {
		t.onChange(snapshot)
	}
}

// setProgress records how many of the total rows a RUNNING scrape has fetched so
// far. It is a no-op once the run has left the RUNNING state (e.g. a stray report
// arriving after cancellation), or once this tracker has moved on to another run,
// so it never resurrects a finished status nor overwrites a newer run's.
func (t *Tracker) setProgress(token string, processed, total int) {
	t.mu.Lock()
	if t.status.State != StateRunning || t.runToken != token {
		t.mu.Unlock()
		return
	}
	t.status.Processed = processed
	t.status.Total = total
	snapshot := t.status
	t.mu.Unlock()

	// 共有の記録も進める。ここでも印の寿命は延びるが、延ばす役目そのものは
	// 心拍（heartbeat）が持つ。進捗の報告が途切れる区間があるため。
	if err := t.store.Update(t.ctx, token, snapshot); err != nil {
		logger.Log.Error().Err(err).Msg("failed to record course import progress")
	}
	t.notify()
}

// Start rejects the call with apperr.Conflict if a run is already in progress.
// Otherwise it transitions to RUNNING, returns that snapshot immediately, and
// runs run in the background against a fresh context.Background() (the request
// context would be cancelled once the mutation response is sent). run is handed
// t.SetProgress so it can report incremental progress while it works.
func (t *Tracker) Start(year int, run func(ctx context.Context, reportProgress func(processed, total int)) (imported, skipped int, err error)) (Status, error) {
	t.mu.Lock()
	if t.stopping {
		t.mu.Unlock()
		return Status{}, apperr.Conflict("サーバー停止中はインポートを開始できません")
	}
	t.mu.Unlock()

	// 実行中かどうかは共有の記録で決める。自分の台のメモリだけを見ていると、
	// 管理者2人が別々の台に当たったときに同じ取り込みが2本走る。
	now := time.Now()
	snapshot := Status{State: StateRunning, Year: year, StartedAt: &now}
	token, acquired, err := t.store.TryStart(t.ctx, snapshot)
	if err != nil {
		// 取れたかどうかが分からないまま走らせない。二重起動は、同じ年度の
		// 授業を2回取り込むという後始末の要る壊れ方になる。
		logger.Log.Error().Err(err).Msg("failed to take the course import lock")
		return Status{}, apperr.Conflict("インポートの状態を確認できませんでした")
	}
	if !acquired {
		return Status{}, apperr.Conflict("既にインポートを実行中です")
	}

	// 心拍の間隔は実行を始める時点で1度だけ読む。goroutine の中で読むと、
	// テストが間隔を差し替えるのと goroutine の起動が競合する（本番では
	// 書き換えないので、これはテストのための取り決め）。
	heartbeatEvery := LockHeartbeatInterval

	t.mu.Lock()
	t.status = snapshot
	t.runToken = token
	t.jobs.Add(1)
	t.mu.Unlock()
	t.notify()

	go func() {
		defer t.jobs.Done()
		bgCtx, cancel := context.WithCancel(t.ctx)
		defer cancel()

		stalled := make(chan struct{})
		stopWatchdog := make(chan struct{})
		stopHeartbeat := make(chan struct{})
		resetStall := make(chan struct{}, 1)
		go func() {
			timer := time.NewTimer(progressStallTimeout)
			defer timer.Stop()
			for {
				select {
				case <-resetStall:
					timer.Stop()
					timer.Reset(progressStallTimeout)
				case <-timer.C:
					close(stalled)
					return
				case <-stopWatchdog:
					return
				}
			}
		}()
		go func() {
			select {
			case <-stalled:
				cancel()
			case <-stopWatchdog:
			}
		}()

		// 実行中の印を、進捗と関係なく延ばし続ける。
		//
		// 以前は進捗の報告でしか延びなかった。スクレイピングを終えた後の一括保存
		// （DBへの書き込み）は報告を出さないので、そこが長引くと印が解け、
		// まだDBへ書いている実行を残したまま別の台が次の取り込みを始められた。
		//
		// 心拍が「もう自分の印ではない」と分かったら、その場でこの実行を止める。
		// 印を持っていない実行がDBを触り続けるのが、二重取り込みの実体だから。
		//
		// 止まったと判断された（stalled）後は延ばさない。延ばし続けると、固まった
		// 実行が印を抱えたままになり、どの台も次を始められなくなる。
		go func() {
			ticker := time.NewTicker(heartbeatEvery)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-stalled:
					return
				case <-ticker.C:
					held, err := t.store.Heartbeat(t.ctx, token)
					if err != nil {
						// 延ばせたか分からない。次の心拍で確かめ直す
						// （ここで止めると、Redis の瞬断で取り込みが落ちる）。
						logger.Log.Error().Err(err).Msg("failed to extend the course import lock")
						continue
					}
					if !held {
						logger.Log.Error().Int("year", year).
							Msg("the course import lock was taken over by another run; aborting this one")
						cancel()
						return
					}
				}
			}
		}()

		reportProgress := func(processed, total int) {
			select {
			case resetStall <- struct{}{}:
			default:
			}
			t.setProgress(token, processed, total)
		}

		imported, skipped, err := run(bgCtx, reportProgress)
		close(stopHeartbeat)
		close(stopWatchdog)
		finished := time.Now()
		if err == nil && bgCtx.Err() != nil {
			err = bgCtx.Err()
		}

		select {
		case <-stalled:
			if err != nil {
				err = fmt.Errorf("no progress for %s; aborted (last error: %w)", progressStallTimeout, err)
			} else {
				err = fmt.Errorf("no progress for %s; aborted", progressStallTimeout)
			}
		default:
		}

		t.mu.Lock()
		startedAt := t.status.StartedAt
		if t.runToken != token {
			// この台で既に次の実行が始まっている（＝印はその実行が持っている）。
			// 共有の記録は token で弾かれるうえ、外すべき印もこちらには無いので、
			// 写しにも記録にも触らずに降りる。
			t.mu.Unlock()
			logger.Log.Warn().Int("year", year).
				Msg("this course import was superseded by a newer run on the same instance; dropping its result")
			return
		}
		if err != nil {
			t.status = Status{State: StateFailed, Year: year, ErrorMessage: err.Error(), StartedAt: startedAt, FinishedAt: &finished}
			final := t.status
			t.runToken = ""
			t.mu.Unlock()
			logger.Log.Error().Err(err).Int("year", year).Msg("course import failed")
			t.finish(token, final)
			return
		}
		t.status = Status{State: StateSucceeded, Year: year, Imported: imported, Skipped: skipped, StartedAt: startedAt, FinishedAt: &finished}
		final := t.status
		t.runToken = ""
		t.mu.Unlock()
		t.finish(token, final)
	}()

	return snapshot, nil
}

// finish は最終状態を共有の記録へ書き、実行中の印を外してから通知する。
//
// 印を外し損ねると、この台が終わっているのに他の台から「既に実行中です」と
// 見え続ける。Redis の実装では印に寿命を持たせてあるので、書き込みに失敗しても
// 寿命が切れれば解ける（その間は取り込みを始められない、で済む）。
//
// 停止用の ctx（t.ctx）ではなく context.Background() を使う。サーバー停止に
// 伴う終了では t.ctx は既に切れているので、そのまま使うと最終状態を
// 書き残せず、状態が RUNNING のまま取り残される。
func (t *Tracker) finish(token string, final Status) {
	if err := t.store.Finish(context.Background(), token, final); err != nil {
		logger.Log.Error().Err(err).Msg("failed to record the course import result")
	}
	t.notify()
}
