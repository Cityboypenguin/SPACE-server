package courseimport

import (
	"context"
	"sync"
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
type Store interface {
	// TryStart は「実行中」を立てる。既に誰かが実行中なら false を返す。
	// 台をまたいで不可分であること。
	TryStart(ctx context.Context, status Status) (bool, error)
	// Update は実行中の状態を書き換える（進捗の報告）。
	// 実行中の印の寿命もここで延ばす。
	Update(ctx context.Context, status Status) error
	// Finish は最終状態を書いて、実行中の印を外す。
	Finish(ctx context.Context, status Status) error
	// Load は現在の状態を返す。誰も実行しておらず記録も無ければ IDLE。
	Load(ctx context.Context) (Status, error)
}

// MemoryStore はプロセス内に状態を持つ Store。
//
// 台が1つの構成（ローカル開発・テスト）向け。台を増やす構成では
// Redis の実装（infra/redis）を使うこと。
type MemoryStore struct {
	mu      sync.Mutex
	status  Status
	running bool
}

var _ Store = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{status: Status{State: StateIdle}}
}

func (s *MemoryStore) TryStart(_ context.Context, status Status) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false, nil
	}
	s.running = true
	s.status = status
	return true, nil
}

func (s *MemoryStore) Update(_ context.Context, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		// 終わった後に届いた報告で、終了状態を実行中へ巻き戻さない。
		return nil
	}
	s.status = status
	return nil
}

func (s *MemoryStore) Finish(_ context.Context, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.status = status
	return nil
}

func (s *MemoryStore) Load(_ context.Context) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, nil
}
