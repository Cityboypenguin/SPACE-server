package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
const sseHistoryTTL = 10 * time.Minute

// 採番のキーには寿命を付けない。
//
// 付けてはいけない。SSE の再接続が運べるのは Last-Event-ID だけで、それは
// 「この利用者向けの採番の、どこまで受け取ったか」を指す。採番が消えて 1 から
// 振り直されると、その番号が何を指すのかが変わってしまう。
//
//	1. 10分イベントが無く、履歴と採番が同時に消える
//	2. 新しいイベントが id=1,2,3 として積まれる
//	3. 切断前に id=57 まで受け取っていたタブが Last-Event-ID: 57 で再接続する
//	4. 履歴には 1..3 しか無いので「57 より新しいものは無い」＝取りこぼす
//
// 履歴だけなら消えてもよい（リプレイを諦めてクライアントが一覧を取り直す）が、
// 採番が消えると「リプレイを諦めるべき状態」だと気づけないまま黙って落ちる。
// 採番は利用者あたり数バイトなので、残し続けても費用にならない。
//
// それでも巻き戻りうる経路は残る（Redis の flush、maxmemory による追い出し）。
// そちらは Replay 側で「採番より先のIDを名乗るクライアント」として弾く。

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

// sseAppendScript は採番と履歴への記録を一続きに行う。
//
// 分けてはいけない。INCR と RPUSH を別々に撃つと、同じ利用者宛の Append が
// 重なったときに「採番の順」と「履歴に並ぶ順」がずれる。
//
//	A: INCR -> 1        B: INCR -> 2
//	B: RPUSH(id=2)      A: RPUSH(id=1)   → 履歴は [2, 1]
//
// こうなると (a) リプレイが id の昇順で出ないのでクライアントの
// Last-Event-ID が小さい方で止まり、次の再接続で配り直しになる、
// (b) 履歴の溢れ判定（LTRIM は並び順の先頭から落とす）が id の古い順で
// なくなるので、新しいイベントの方が先に捨てられうる。
//
// Redis はスクリプトを1つずつ実行するので、この中に入れれば採番の順と
// 並び順は必ず一致する。
//
// イベントの JSON を Lua で組み立てずに済ませるため、呼び出し側が
// 「id の手前まで」と「id の後ろ全部」に割った文字列を渡し、ここでは採番した
// 数字を挟むだけにしてある（Lua で cjson を通すと、空のオブジェクトや数値の
// 表記が Go 側と変わりうる）。
var sseAppendScript = redis.NewScript(`
local id = redis.call("INCR", KEYS[1])
redis.call("RPUSH", KEYS[2], ARGV[1] .. id .. ARGV[2])
redis.call("LTRIM", KEYS[2], -tonumber(ARGV[3]), -1)
redis.call("EXPIRE", KEYS[2], tonumber(ARGV[4]))
return id
`)

// splitEventJSON は採番前のイベントを「id の手前」と「id の後ろ」に割る。
//
// sse.Event をそのまま Marshal すると id の値まで入ってしまうので、id を持たない
// 形で組み立てて、先頭の `{` を `,` に置き換えたものを後ろ半分にする。
// 結果として Lua 側で繋がるのは
//
//	{"id": + 12 + ,"type":"...","data":{...},"time":""}
//
// で、sse.Event を Marshal したものと同じ並び（id, type, data, time）になる。
func splitEventJSON(eventType string, data map[string]any) (head, tail string, err error) {
	body, err := json.Marshal(struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
		Time string         `json:"time"`
	}{Type: eventType, Data: data})
	if err != nil {
		return "", "", fmt.Errorf("failed to encode an SSE event: %w", err)
	}
	// body は必ず `{"type":...}` の形。先頭の `{` を `,` に読み替える。
	return `{"id":`, "," + string(body[1:]), nil
}

func (s *SSEStore) Append(ctx context.Context, userID int64, eventType string, data map[string]any) (sse.Event, error) {
	head, tail, err := splitEventJSON(eventType, data)
	if err != nil {
		return sse.Event{}, err
	}

	// 採番と記録は不可分（sseAppendScript のコメント参照）。失敗したときは
	// 何も起きていないので、イベントを配らずに返す。以前は INCR だけ済ませて
	// 履歴の書き込み失敗を握り潰していたが、その道は「採番は進んだのに履歴に
	// 無い」状態を作るだけで、どのみち Redis が不調なら配信（SSEFanout）も
	// 通らない。
	id, err := sseAppendScript.Run(ctx, s.client,
		[]string{sseCounterKey(userID), sseHistoryKey(userID)},
		head, tail, sseHistorySize, int(sseHistoryTTL.Seconds()),
	).Int64()
	if err != nil {
		return sse.Event{}, fmt.Errorf("failed to record an SSE event: %w", err)
	}

	return sse.Event{ID: int(id), Type: eventType, Data: data}, nil
}

func (s *SSEStore) Replay(ctx context.Context, userID int64, lastEventID int) ([]sse.Event, error) {
	if lastEventID < 0 {
		return nil, nil
	}

	// 履歴と採番を1往復で見る。採番も見るのは、クライアントが名乗るIDが採番より
	// 先に居ないかを確かめるため（MemoryStore.Replay と同じ判定）。
	pipe := s.client.Pipeline()
	rangeCmd := pipe.LRange(ctx, sseHistoryKey(userID), 0, -1)
	counterCmd := pipe.Get(ctx, sseCounterKey(userID))
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to read the SSE history: %w", err)
	}

	raws, err := rangeCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to read the SSE history: %w", err)
	}

	// 採番のキーが無ければ 0（まだ1件も振っていない）として扱う。
	lastIssued := 0
	if raw, err := counterCmd.Result(); err == nil {
		if n, convErr := strconv.Atoi(raw); convErr == nil {
			lastIssued = n
		}
	} else if !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to read the SSE counter: %w", err)
	}

	if lastEventID > lastIssued {
		// 名乗るIDが採番より先に居る。採番は消さない運用なので、ここへ来るのは
		// Redis ごと空になった後（flush、maxmemory での追い出し）。履歴に残って
		// いるものを「新しい」と誤判定すると取りこぼすので、リプレイを諦める。
		// 通知の本体は DB にあるので、クライアントが接続後に一覧を取り直す。
		sse.LogReplayAfterRestart(userID, lastEventID, lastIssued+1)
		return nil, nil
	}

	if len(raws) == 0 {
		// 履歴が無い（無接続のまま寿命が切れた、など）。
		sse.LogReplayNoHistory(userID, lastEventID)
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
