package courseimport

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Store は取り込みの状態を置く場所。
//
// 台を1つしか立てないうちは、状態をプロセスのメモリに置いておけば足りた。台を
// 増やすと2つのことが壊れる。
//
//   - 排他が効かない。「既に実行中です」の判定が自分の台のメモリしか見ないので、
//     管理者2人が別々の台に当たれば、同じ年度の取り込みが同時に2本走る。
//   - 進捗が見えない。管理画面が取り込みを始めた台とは別の台へ繋がると、
//     状態は IDLE のままに見える。
//
// どちらも「片方の台では正常に見える」形で壊れるので、動かしてみて気づくのは難しい。
//
// TryStart が排他の要。ここだけは台をまたいで不可分でなければならない。
//
// 書き込む口（Update / Heartbeat / Finish）が token を取るのは、「どの実行か」を
// 台のIDで表してはいけないため。印が寿命で解けた後、同じ台で次の実行が始まると、
// 古い実行の報告も台のIDでは一致してしまい、新しい実行の状態を上書きできてしまう。
// token は実行ごとに新しく発行すること。
type Store interface {
	// TryStart は「実行中」を立て、その実行を表す token を返す。
	// 既に誰かが実行中なら ok=false（token は空）。
	// 台をまたいで不可分であること。
	TryStart(ctx context.Context, status Status) (token string, ok bool, err error)
	// Update は token が今の持ち主であるときだけ状態を書き換え（進捗の報告）、
	// 実行中の印の寿命を延ばす。持ち主でなければ何もしない。
	Update(ctx context.Context, token string, status Status) error
	// Heartbeat は token が今の持ち主であるときだけ印の寿命を延ばし、true を返す。
	//
	// 進捗の報告と別に要るのは、取り込みが「報告するものが無いまま長く走る」
	// 区間を持つため（スクレイピング後の一括保存）。そこで寿命が切れると、
	// まだDBへ書いている最中の実行を残したまま別の台が次の実行を始められる。
	//
	// false が返ったら、その実行はもう印を持っていない。呼び出し側は
	// **その実行を止めること**（DBへ書き続けてはいけない）。
	Heartbeat(ctx context.Context, token string) (bool, error)
	// Finish は token が今の持ち主であるときだけ最終状態を書き、印を外す。
	Finish(ctx context.Context, token string, status Status) error
	// Load は現在の状態を返す。誰も実行しておらず記録も無ければ IDLE。
	Load(ctx context.Context) (Status, error)
}

// LockHeartbeatInterval は実行中の印を延ばしに行く間隔。
//
// Store の実装は、印の寿命をこの間隔より十分長く（数回分は落としても解けない
// くらいに）取ること。infra/redis 側にその関係を確かめるテストがある。
//
// const ではなく var なのはテストから短くするため（本番で書き換えてはいけない）。
// 心拍を1分待つテストは、確かめたい性質のわりに遅すぎる。
var LockHeartbeatInterval = time.Minute

// MemoryStore はプロセス内に状態を持つ Store。
//
// 台が1つの構成（ローカル開発・テスト）向け。台を増やす構成では
// Redis の実装（infra/redis）を使うこと。
type MemoryStore struct {
	mu      sync.Mutex
	status  Status
	running bool
	// token は今の実行の合言葉、runs はその採番。
	token string
	runs  int
}

var _ Store = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{status: Status{State: StateIdle}}
}

func (s *MemoryStore) TryStart(_ context.Context, status Status) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return "", false, nil
	}
	// 台が1つでも token は実行ごとに変える。Redis の実装と規則を揃えておかないと、
	// 「メモリでは通るのに本番では通らない」呼び出し方をテストが見逃す。
	s.runs++
	s.token = fmt.Sprintf("memory-%d", s.runs)
	s.running = true
	s.status = status
	return s.token, true, nil
}

func (s *MemoryStore) Update(_ context.Context, token string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.holds(token) {
		// 終わった後や、別の実行に入れ替わった後に届いた報告では書かない。
		return nil
	}
	s.status = status
	return nil
}

func (s *MemoryStore) Heartbeat(_ context.Context, token string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.holds(token), nil
}

func (s *MemoryStore) Finish(_ context.Context, token string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.holds(token) {
		return nil
	}
	s.running = false
	s.token = ""
	s.status = status
	return nil
}

// holds は token が今の実行のものか。錠を持った状態で呼ぶこと。
func (s *MemoryStore) holds(token string) bool {
	return s.running && token != "" && token == s.token
}

func (s *MemoryStore) Load(_ context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, nil
}
