package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis は1本のテスト専用の Redis を立てる。
//
// 偽物（インメモリ実装）ではなく Redis を喋るものを使うのは、ここで確かめたいのが
// 「Redis 側でどう実行されるか」そのものだから。Lua の中で採番と記録が一続きに
// 走ること、寿命がどのキーに付くことは、偽物の口を作ってしまうと何も確かめられない。
func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return srv, client
}

// 積んだものが、そのまま読み戻せること。
//
// 当たり前に見えるが、イベントの JSON は Go で組み立てた前後半を Lua が採番を
// 挟んで繋いでいる（splitEventJSON / sseAppendScript）。繋ぎ方を間違えると
// 壊れた JSON が履歴に入り、Replay が黙って1件飛ばす形で現れる。
func TestSSEStore_AppendedEventsComeBackFromReplay(t *testing.T) {
	_, client := newTestRedis(t)
	store := NewSSEStore(client)
	ctx := context.Background()

	first, err := store.Append(ctx, 1, "notifications_changed", map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	second, err := store.Append(ctx, 1, "room_changed", map[string]any{"roomID": "room-1"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if first.ID != 1 || second.ID != 2 {
		t.Fatalf("ids = %d, %d; want 1, 2", first.ID, second.ID)
	}

	missed, err := store.Replay(ctx, 1, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(missed) != 2 {
		t.Fatalf("replayed %d events, want 2", len(missed))
	}
	if missed[0].ID != 1 || missed[0].Type != "notifications_changed" {
		t.Fatalf("first replayed event = %+v", missed[0])
	}
	if missed[1].ID != 2 || missed[1].Data["roomID"] != "room-1" {
		t.Fatalf("second replayed event = %+v", missed[1])
	}
}

// 採番のキーに寿命を付けないこと。履歴には付けること。
//
// 以前は両方に10分を付けていた。10分イベントが無いと採番も消えるので、次の
// イベントが id=1 に戻る。切断前に id=57 まで受け取っていたタブが
// Last-Event-ID: 57 で戻ってくると、履歴の 1..3 は「57 より古い」と判定されて
// 何もリプレイされない（＝取りこぼす）。履歴は消えてよいが、採番は消えてはいけない。
func TestSSEStore_TheCounterNeverExpires(t *testing.T) {
	srv, client := newTestRedis(t)
	store := NewSSEStore(client)
	ctx := context.Background()

	if _, err := store.Append(ctx, 1, "notifications_changed", nil); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if ttl := srv.TTL(sseCounterKey(1)); ttl != 0 {
		t.Fatalf("採番のキーに寿命が付いている（%s）。消えるとIDが1へ巻き戻る", ttl)
	}
	if ttl := srv.TTL(sseHistoryKey(1)); ttl != sseHistoryTTL {
		t.Fatalf("履歴の寿命 = %s, want %s", ttl, sseHistoryTTL)
	}

	// 履歴だけが寿命で消えても、採番は続きから振られる。
	srv.FastForward(sseHistoryTTL + 1)
	ev, err := store.Append(ctx, 1, "notifications_changed", nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if ev.ID != 2 {
		t.Fatalf("履歴の失効後のID = %d, want 2（採番は巻き戻らない）", ev.ID)
	}
}

// 採番より先のIDを名乗るクライアントには、何もリプレイしないこと。
//
// 採番を消さない運用でも、Redis ごと空になれば（flush、maxmemory による追い出し）
// 巻き戻りうる。そのとき履歴に残る 1..3 を「57 より新しい」と誤判定すると、
// 本当は見ていないものを既読扱いしてしまう。諦めれば、クライアントは接続後に
// 通知一覧を取り直して追いつける。
func TestSSEStore_SkipsReplayWhenTheCursorIsAheadOfTheCounter(t *testing.T) {
	_, client := newTestRedis(t)
	store := NewSSEStore(client)
	ctx := context.Background()

	for range 3 {
		if _, err := store.Append(ctx, 1, "notifications_changed", nil); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	missed, err := store.Replay(ctx, 1, 57)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(missed) != 0 {
		t.Fatalf("replayed %d events, want 0（採番より先を名乗るカーソルは信用しない）", len(missed))
	}

	// 巻き戻っていない普通の再接続は、これまでどおりリプレイされる。
	if missed, err = store.Replay(ctx, 1, 1); err != nil || len(missed) != 2 {
		t.Fatalf("正常な再接続: %d events, err=%v; want 2", len(missed), err)
	}
}

// 同じ利用者へ同時に積んでも、採番の順と履歴の並びがずれないこと。
//
// 以前は INCR と RPUSH を別々に撃っていたので、A が先に採番して B が先に
// 書き込む、が起こりえた。履歴が [2, 1] の順になると、リプレイがIDの昇順で
// 出ないので、受け取ったクライアントの Last-Event-ID が小さい方で止まり、
// 次の再接続で配り直しになる。
func TestSSEStore_ConcurrentAppendsKeepIDsAndOrderTogether(t *testing.T) {
	_, client := newTestRedis(t)
	store := NewSSEStore(client)
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := store.Append(ctx, 1, "notifications_changed", map[string]any{"i": i}); err != nil {
				t.Errorf("Append: %v", err)
			}
		}(i)
	}
	wg.Wait()

	missed, err := store.Replay(ctx, 1, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(missed) != n {
		t.Fatalf("replayed %d events, want %d", len(missed), n)
	}
	for i, ev := range missed {
		if ev.ID != i+1 {
			t.Fatalf("履歴の %d 番目のID = %d, want %d（採番の順と並びがずれている）", i, ev.ID, i+1)
		}
	}
}

// 履歴は上限を超えたら古い方から落ちること。
func TestSSEStore_KeepsOnlyTheMostRecentEvents(t *testing.T) {
	_, client := newTestRedis(t)
	store := NewSSEStore(client)
	ctx := context.Background()

	for range sseHistorySize + 10 {
		if _, err := store.Append(ctx, 1, "notifications_changed", nil); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	missed, err := store.Replay(ctx, 1, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(missed) != sseHistorySize {
		t.Fatalf("history size = %d, want %d", len(missed), sseHistorySize)
	}
	if missed[0].ID != 11 {
		t.Fatalf("oldest kept id = %d, want 11（古い方から落ちる）", missed[0].ID)
	}
}

// Lua が組み立てる JSON が、Go で sse.Event をそのまま変換したものと一致すること。
//
// 履歴に入るのは Lua が前後半を繋いだ文字列で、読み戻すのは Go の
// json.Unmarshal。両者の形がずれると、静かに欠けたフィールドで復元される
// （例えば data が落ちれば、リプレイされた通知だけ中身が空になる）。
func TestSplitEventJSON_MatchesMarshalingTheWholeEvent(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		data      map[string]any
	}{
		{"データなし", "notifications_changed", nil},
		{"空のデータ", "notifications_changed", map[string]any{}},
		{"値が入っている", "room_changed", map[string]any{"roomID": "room-1", "n": 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head, tail, err := splitEventJSON(tc.eventType, tc.data)
			if err != nil {
				t.Fatalf("splitEventJSON: %v", err)
			}
			got := head + "42" + tail

			want, err := marshalEvent(sse.Event{ID: 42, Type: tc.eventType, Data: tc.data})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got != want {
				t.Fatalf("組み立てた JSON が違う\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func marshalEvent(ev sse.Event) (string, error) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return string(raw), nil
}

// 一度も通知を受けていない利用者の再接続でも、エラーにならず素通りすること。
// （履歴も採番も無い＝キーが両方無い状態。Replay は1往復でその2つを見るので、
// 「無い」を素直に扱えているかはここでしか通らない。）
func TestSSEStore_ReplayForAnUnknownUserIsEmpty(t *testing.T) {
	_, client := newTestRedis(t)
	store := NewSSEStore(client)
	ctx := context.Background()

	missed, err := store.Replay(ctx, 999, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(missed) != 0 {
		t.Fatalf("replayed %d events, want 0", len(missed))
	}

	// 初回接続（-1）も同じ。
	if missed, err = store.Replay(ctx, 999, -1); err != nil || len(missed) != 0 {
		t.Fatalf("初回接続: %d events, err=%v", len(missed), err)
	}
}
