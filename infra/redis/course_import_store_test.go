package redis

import (
	"context"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
)

func running(year int) courseimport.Status {
	return courseimport.Status{State: courseimport.StateRunning, Year: year}
}

// 印は1台しか取れないこと（排他の要）。
func TestCourseImportStore_OnlyOneInstanceCanStart(t *testing.T) {
	_, client := newTestRedis(t)
	ctx := context.Background()
	a := NewCourseImportStore(client, "instance-a")
	b := NewCourseImportStore(client, "instance-b")

	token, started, err := a.TryStart(ctx, running(2026))
	if err != nil || !started {
		t.Fatalf("1台目が始められない: started=%v err=%v", started, err)
	}
	if _, started, err = b.TryStart(ctx, running(2026)); err != nil {
		t.Fatalf("TryStart: %v", err)
	}
	if started {
		t.Fatal("2台目まで始められてしまった（同じ年度の取り込みが二重に走る）")
	}

	// 1台目が終えれば印が外れ、次が始められる。
	if err := a.Finish(ctx, token, courseimport.Status{State: courseimport.StateSucceeded, Year: 2026}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, started, err = b.TryStart(ctx, running(2026)); err != nil || !started {
		t.Fatalf("終了後に始められない: started=%v err=%v", started, err)
	}
}

// 合言葉が実行ごとに変わること。
//
// 台のIDを合言葉にしていると、印が寿命で解けた後に**同じ台で**次の実行が始まった
// とき、古い実行の報告も一致してしまう。台をまたいだ取り違えは防げても、
// 1台構成では何も守られていない状態になる。
func TestCourseImportStore_TokensDifferPerRun(t *testing.T) {
	srv, client := newTestRedis(t)
	ctx := context.Background()
	store := NewCourseImportStore(client, "instance-a")

	first, _, err := store.TryStart(ctx, running(2025))
	if err != nil {
		t.Fatalf("TryStart: %v", err)
	}

	// 古い実行が固まり、印が寿命で解ける。同じ台で次の実行が始まる。
	srv.FastForward(courseImportLockTTL + 1)
	second, started, err := store.TryStart(ctx, running(2026))
	if err != nil || !started {
		t.Fatalf("同じ台で再開できない: started=%v err=%v", started, err)
	}
	if first == second {
		t.Fatal("実行が違うのに合言葉が同じ（古い実行が新しい実行になりすませる）")
	}

	// 古い実行の報告も終了も、新しい実行の状態を動かせない。
	if err := store.Update(ctx, first, courseimport.Status{State: courseimport.StateRunning, Year: 2025, Processed: 900}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := store.Finish(ctx, first, courseimport.Status{State: courseimport.StateSucceeded, Year: 2025}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.State != courseimport.StateRunning || got.Year != 2026 || got.Processed != 0 {
		t.Fatalf("古い実行に上書きされた: %+v", got)
	}
	if held, err := store.Heartbeat(ctx, second); err != nil || !held {
		t.Fatalf("新しい実行が印を失っている: held=%v err=%v", held, err)
	}
}

// 寿命が切れて別の台が引き継いだあと、遅れてきた古い台の進捗報告が
// 新しい実行の状態を上書きしないこと。
func TestCourseImportStore_UpdateDoesNothingOnceTheLockIsSomeoneElses(t *testing.T) {
	srv, client := newTestRedis(t)
	ctx := context.Background()
	old := NewCourseImportStore(client, "instance-old")
	fresh := NewCourseImportStore(client, "instance-new")

	oldToken, _, err := old.TryStart(ctx, running(2025))
	if err != nil {
		t.Fatalf("TryStart: %v", err)
	}

	srv.FastForward(courseImportLockTTL + 1)
	if _, started, err := fresh.TryStart(ctx, running(2026)); err != nil || !started {
		t.Fatalf("引き継げない: started=%v err=%v", started, err)
	}

	if err := old.Update(ctx, oldToken, courseimport.Status{State: courseimport.StateRunning, Year: 2025, Processed: 900}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := fresh.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Year != 2026 || got.Processed != 0 {
		t.Fatalf("古い実行の進捗で塗り替えられた: %+v", got)
	}
}

// 遅れてきた古い台の「終わりました」が、走っている実行の状態を上書きしないこと。
//
// これが起きると、管理画面には SUCCEEDED と出るのに取り込みはまだ走っている、
// という食い違いになる。
func TestCourseImportStore_FinishDoesNotOverwriteAnotherRun(t *testing.T) {
	srv, client := newTestRedis(t)
	ctx := context.Background()
	old := NewCourseImportStore(client, "instance-old")
	fresh := NewCourseImportStore(client, "instance-new")

	oldToken, _, err := old.TryStart(ctx, running(2025))
	if err != nil {
		t.Fatalf("TryStart: %v", err)
	}
	srv.FastForward(courseImportLockTTL + 1)
	if _, _, err := fresh.TryStart(ctx, running(2026)); err != nil {
		t.Fatalf("TryStart: %v", err)
	}

	if err := old.Finish(ctx, oldToken, courseimport.Status{State: courseimport.StateSucceeded, Year: 2025}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got, err := fresh.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.State != courseimport.StateRunning || got.Year != 2026 {
		t.Fatalf("走っている実行の状態が古い台の結果で上書きされた: %+v", got)
	}

	// 印も外れていないこと（外れていれば三台目が割り込める）。
	if _, started, err := NewCourseImportStore(client, "instance-third").TryStart(ctx, running(2027)); err != nil || started {
		t.Fatalf("古い台の Finish が印まで外した: started=%v err=%v", started, err)
	}
}

// 心拍で印の寿命が延びること。
//
// 進捗の報告と別に要るのは、スクレイピングを終えた後の一括保存が報告を出さない
// ため。そこで寿命が切れると、まだDBへ書いている実行を残したまま別の台が
// 次の取り込みを始められる。
func TestCourseImportStore_HeartbeatExtendsTheLockWithoutTouchingTheStatus(t *testing.T) {
	srv, client := newTestRedis(t)
	ctx := context.Background()
	store := NewCourseImportStore(client, "instance-a")

	token, _, err := store.TryStart(ctx, courseimport.Status{State: courseimport.StateRunning, Year: 2026, Processed: 42})
	if err != nil {
		t.Fatalf("TryStart: %v", err)
	}

	srv.FastForward(courseImportLockTTL - time.Minute)
	held, err := store.Heartbeat(ctx, token)
	if err != nil || !held {
		t.Fatalf("Heartbeat: held=%v err=%v", held, err)
	}
	if ttl := srv.TTL(courseImportLockKey); ttl != courseImportLockTTL {
		t.Fatalf("印の寿命 = %s, want %s", ttl, courseImportLockTTL)
	}

	// 状態には触らない（延ばすためだけに書き直さない）。
	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Processed != 42 || got.Year != 2026 {
		t.Fatalf("心拍が状態を書き換えた: %+v", got)
	}
}

// 印を失った実行の心拍が false を返すこと。呼び出し側（Tracker）はこれを見て
// その実行を止める。
func TestCourseImportStore_HeartbeatReportsALostLock(t *testing.T) {
	srv, client := newTestRedis(t)
	ctx := context.Background()
	old := NewCourseImportStore(client, "instance-old")
	fresh := NewCourseImportStore(client, "instance-new")

	oldToken, _, err := old.TryStart(ctx, running(2025))
	if err != nil {
		t.Fatalf("TryStart: %v", err)
	}
	srv.FastForward(courseImportLockTTL + 1)
	if _, _, err := fresh.TryStart(ctx, running(2026)); err != nil {
		t.Fatalf("TryStart: %v", err)
	}

	held, err := old.Heartbeat(ctx, oldToken)
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if held {
		t.Fatal("奪われた印を、まだ持っていると答えた（古い実行がDBを触り続ける）")
	}
}

// 進捗の報告でも寿命が延びること。
func TestCourseImportStore_UpdateExtendsTheLock(t *testing.T) {
	srv, client := newTestRedis(t)
	ctx := context.Background()
	store := NewCourseImportStore(client, "instance-a")

	token, _, err := store.TryStart(ctx, running(2026))
	if err != nil {
		t.Fatalf("TryStart: %v", err)
	}

	srv.FastForward(courseImportLockTTL - time.Minute)
	if err := store.Update(ctx, token, courseimport.Status{State: courseimport.StateRunning, Year: 2026, Processed: 10}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if ttl := srv.TTL(courseImportLockKey); ttl != courseImportLockTTL {
		t.Fatalf("印の寿命 = %s, want %s", ttl, courseImportLockTTL)
	}

	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Processed != 10 {
		t.Fatalf("進捗が保存されていない: %+v", got)
	}
}

// 印を持ったまま台が落ちた状態を、管理画面が「実行中」のままにしないこと。
func TestCourseImportStore_ReportsAbandonedRunsAsFailed(t *testing.T) {
	srv, client := newTestRedis(t)
	ctx := context.Background()
	store := NewCourseImportStore(client, "instance-a")

	if _, _, err := store.TryStart(ctx, running(2026)); err != nil {
		t.Fatalf("TryStart: %v", err)
	}
	srv.FastForward(courseImportLockTTL + 1)

	got, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.State != courseimport.StateFailed || got.ErrorMessage == "" {
		t.Fatalf("見放された実行が実行中のままになっている: %+v", got)
	}
}

// 印の寿命が、心拍を何回か落としても解けない長さであること。
//
// この2つは別のパッケージにあり、関係はコメントでしか繋がっていない。片方だけ
// 動かすと「心拍を出しているのに印が切れる」という、動かしてみるまで分からない
// 壊れ方になるので、ここで固定しておく。
func TestCourseImportLockOutlivesSeveralHeartbeats(t *testing.T) {
	if courseImportLockTTL < courseimport.LockHeartbeatInterval*5 {
		t.Fatalf("印の寿命 %s は心拍の間隔 %s に対して短すぎる（数回落としただけで解ける）",
			courseImportLockTTL, courseimport.LockHeartbeatInterval)
	}
}
