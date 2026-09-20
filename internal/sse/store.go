package sse

import (
	"context"
	"sync"
	"time"
)

// Store は SSE のイベントの採番と履歴の置き場。
//
// 台を1つしか立てないうちは、プロセスのメモリで足りていた。台を増やすと採番が壊れる。
// 同じ利用者のタブAが1台目、タブBが2台目へ繋がると、それぞれが 1 から採番するので
// 同じIDの別イベントが2種類できる。SSE の再接続は Last-Event-ID（最後に受け取ったID）
// しか運べないので、リプレイの起点が狂い、取りこぼしや二重配信になる。
//
// 履歴も同じ理由で共有する必要がある。1台目で起きたイベントは、2台目へ再接続した
// タブからは「履歴が無い」ことになり、リプレイされない。
type Store interface {
	// Append は userID 向けのイベントを採番して履歴へ積み、採番後の Event を返す。
	Append(ctx context.Context, userID int64, eventType string, data map[string]any) (Event, error)
	// Replay は lastEventID より新しい履歴を返す。lastEventID が負なら何も返さない。
	Replay(ctx context.Context, userID int64, lastEventID int) ([]Event, error)
	// ReleaseIfIdle は「接続が1本も無くなった利用者」の履歴を捨ててよいか判断する。
	// 接続の有無は Broker しか知らないので、判断材料として渡す。
	ReleaseIfIdle(ctx context.Context, connected func(userID int64) bool)
}

// userHistory は1ユーザーぶんのイベント履歴と、そのユーザー向けの採番、
// 最後に書き足した時刻。
// 時刻を持つのは、接続が無くなったユーザーの履歴を捨ててよいか判断するため。
//
// nextID がここ（ユーザー単位）にあるのは、履歴がユーザー単位だから。
// 以前は全ユーザー共通の連番を持っていたので、
//
//	利用者A のイベント: id=1, id=7, id=23, ...
//
// のように**自分宛のID列が飛び飛び**になっていた。間の 2..6 は他人宛に消費された
// ぶんで、A の履歴には最初から存在しない。それでも SSE の仕様上 Last-Event-ID は
// 「最後に受け取った id」しか運べないので、受け手からは「欠番＝取りこぼし」と
// 区別がつかない。実際サーバ側のリプレイ判定（hist[0].ID > lastEventID+1）も
// この欠番で誤爆し、正常な再接続のたびに「取りこぼしたかもしれない」警告を
// 吐いていた。IDを配る単位と、履歴を持つ単位は揃っていなければならない。
type userHistory struct {
	events []Event
	// nextID は次に払い出すイベントID。1 から始まり、このユーザー宛の
	// Append のたびに1つずつ増える（＝欠番が出ない）。
	nextID    int
	updatedAt time.Time
}

// MemoryStore はプロセス内に採番と履歴を持つ Store。台が1つの構成向け。
type MemoryStore struct {
	mu      sync.Mutex
	history map[int64]*userHistory

	// now はテストで時間を進めるための差し替え口（履歴の期限切れを待たずに試すため）。
	now func() time.Time
	// lastSweep は履歴の掃除を最後に走らせた時刻。
	lastSweep time.Time
}

var _ Store = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{history: make(map[int64]*userHistory), now: time.Now}
}

func (s *MemoryStore) Append(_ context.Context, userID int64, eventType string, data map[string]any) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	h := s.history[userID]
	if h == nil {
		h = &userHistory{nextID: 1}
		s.history[userID] = h
	}
	id := h.nextID
	h.nextID++

	ev := Event{ID: id, Type: eventType, Data: data}
	h.events = append(h.events, ev)
	if len(h.events) > historySize {
		h.events = h.events[len(h.events)-historySize:]
	}
	h.updatedAt = s.now()
	return ev, nil
}

func (s *MemoryStore) Replay(_ context.Context, userID int64, lastEventID int) ([]Event, error) {
	if lastEventID < 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	h := s.history[userID]
	switch {
	case h == nil:
		// このユーザーの履歴そのものが無い。サーバー再起動直後か、無接続のまま
		// historyTTL を過ぎて掃除された後。どちらもリプレイできるものは無い。
		// 通知の本体は DB にあるので、クライアントが接続後に一覧を取り直す。
		// 珍しくない（TTL 超えの切断は日常的に起きる）ので警告にはしない。
		logReplayNoHistory(userID, lastEventID)
		return nil, nil
	case lastEventID >= h.nextID:
		// クライアントが名乗るIDが、このユーザーの採番より先に居る。
		// ＝サーバーが再起動して採番が 1 に戻った後、既に何件か配り直している。
		// 採番はユーザー単位なので、他人宛の配信でここへ来ることはない。
		logReplayAfterRestart(userID, lastEventID, h.nextID)
		return nil, nil
	default:
		return SelectMissed(userID, lastEventID, h.events), nil
	}
}

func (s *MemoryStore) ReleaseIfIdle(_ context.Context, connected func(int64) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if now.Sub(s.lastSweep) < historySweepInterval {
		return
	}
	s.lastSweep = now
	for userID, h := range s.history {
		if connected(userID) {
			continue
		}
		if now.Sub(h.updatedAt) >= historyTTL {
			delete(s.history, userID)
		}
	}
}
