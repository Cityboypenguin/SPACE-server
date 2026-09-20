package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/redis/go-redis/v9"
)

// sseHistorySize は1利用者あたり何件の履歴を残すか。
// internal/sse の historySize と同じ考え方（切断中に起きたぶんを再接続で配り直す）。
const sseHistorySize = 100

// sseHistoryTTL は接続が無い利用者の履歴を保持する時間。
//
// プロセス内の実装では「接続が1本も無い利用者」を見て捨てていたが、台が複数ある
// と「この台に繋がっていない」だけでは捨てる根拠にならない（別の台に繋がって
// いるかもしれない）。Redis の寿命に任せ、書き足すたびに延ばす。
//
// 採番のキーにも同じ寿命を付ける。履歴より先に採番が消えると、まだ履歴に残って
// いるIDを二度払い出すことになる。
const sseHistoryTTL = 10 * time.Minute

// SSEStore は SSE の採番と履歴を Redis に置く sse.Store。
//
// 採番を共有するのが要。同じ利用者のタブAが1台目、タブBが2台目へ繋がると、
// プロセス内の採番ではそれぞれが 1 から数えるので、同じIDの別イベントが
// できてしまう。SSE の再接続は Last-Event-ID（最後に受け取ったID）しか運べない
// ので、リプレイの起点が狂って取りこぼしや二重配信になる。
type SSEStore struct {
	client *redis.Client
}

var _ sse.Store = (*SSEStore)(nil)

func NewSSEStore(client *redis.Client) *SSEStore {
	return &SSEStore{client: client}
}

func sseCounterKey(userID int64) string { return fmt.Sprintf("sse:seq:%d", userID) }
func sseHistoryKey(userID int64) string { return fmt.Sprintf("sse:history:%d", userID) }

func (s *SSEStore) Append(ctx context.Context, userID int64, eventType string, data map[string]any) (sse.Event, error) {
	// INCR は台をまたいで不可分なので、これだけで採番が1本に保たれる。
	id, err := s.client.Incr(ctx, sseCounterKey(userID)).Result()
	if err != nil {
		return sse.Event{}, fmt.Errorf("failed to allocate an SSE event id: %w", err)
	}

	ev := sse.Event{ID: int(id), Type: eventType, Data: data}
	raw, err := json.Marshal(ev)
	if err != nil {
		return sse.Event{}, fmt.Errorf("failed to encode an SSE event: %w", err)
	}

	// 履歴は末尾に足して古い方から落とす。採番と履歴の寿命は同時に延ばす
	// （採番だけ先に消えると、まだ履歴に残っているIDを二度払い出す）。
	pipe := s.client.TxPipeline()
	pipe.RPush(ctx, sseHistoryKey(userID), raw)
	pipe.LTrim(ctx, sseHistoryKey(userID), -sseHistorySize, -1)
	pipe.Expire(ctx, sseHistoryKey(userID), sseHistoryTTL)
	pipe.Expire(ctx, sseCounterKey(userID), sseHistoryTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		// 採番は済んでいるので、配ること自体は正しい。履歴に残らないぶん
		// 再接続時のリプレイから漏れるだけなので、失敗としては返さない。
		logger.Log.Error().Err(err).
			Int64("userID", userID).
			Int("eventID", ev.ID).
			Msg("failed to record an SSE event in the history; it will not be replayed on reconnect")
	}
	return ev, nil
}

func (s *SSEStore) Replay(ctx context.Context, userID int64, lastEventID int) ([]sse.Event, error) {
	if lastEventID < 0 {
		return nil, nil
	}

	raws, err := s.client.LRange(ctx, sseHistoryKey(userID), 0, -1).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to read the SSE history: %w", err)
	}
	if len(raws) == 0 {
		// 履歴が無い（無接続のまま寿命が切れた、など）。通知の本体は DB にあるので、
		// クライアントが接続後に一覧を取り直す。
		return nil, nil
	}

	hist := make([]sse.Event, 0, len(raws))
	for _, raw := range raws {
		var ev sse.Event
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			// 1件壊れていてもリプレイ全体は諦めない。
			logger.Log.Error().Err(err).
				Int64("userID", userID).
				Msg("failed to parse a stored SSE event; skipping it")
			continue
		}
		hist = append(hist, ev)
	}

	return sse.SelectMissed(userID, lastEventID, hist), nil
}

// ReleaseIfIdle は何もしない。
//
// 履歴の寿命は Redis の EXPIRE に任せてあるので、掃除する相手が居ない。
// 「この台に繋がっていない」を根拠に消してしまうと、別の台に繋がっている
// 利用者の履歴まで巻き込む。
func (s *SSEStore) ReleaseIfIdle(context.Context, func(int64) bool) {}
