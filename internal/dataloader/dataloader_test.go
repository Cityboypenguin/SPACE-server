package dataloader

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ここで確かめたいのは1つだけ。「N 件解決してもバッチ関数の呼び出しは1回」。
// DataLoader を足しても、リゾルバが1件ずつ Load していたら N+1 は消えないので、
// 呼び出し回数を数える fake を通してローダーごとに押さえる。
//
// 各 fake は「渡されたキー」も記録する。まとめて渡ってきていること（＝1クエリで
// 引ける形になっていること）まで見ないと、「1回しか呼ばれていないが実は1件ずつ
// 引いている」実装を見逃す。

// --- fake use cases -------------------------------------------------------
//
// Go のインターフェースは構造的に満たされるので、同じシグネチャの UseCases
// フィールド（媒体4種など）は1つのアダプタ型を共有できる。
// 関数型にしておくと、テストごとに戻り値だけ差し替えられる。

type recorder struct {
	mu    sync.Mutex
	calls int
	keys  [][]int64
}

func (r *recorder) record(ids []int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	copied := append([]int64(nil), ids...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	r.keys = append(r.keys, copied)
}

func (r *recorder) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *recorder) lastKeys() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.keys) == 0 {
		return nil
	}
	return r.keys[len(r.keys)-1]
}

type sliceFn[V any] func(context.Context, []int64) ([]V, error)

func (f sliceFn[V]) Execute(ctx context.Context, ids []int64) ([]V, error) { return f(ctx, ids) }

type mapFn[V any] func(context.Context, []int64) (map[int64]V, error)

func (f mapFn[V]) Execute(ctx context.Context, ids []int64) (map[int64]V, error) {
	return f(ctx, ids)
}

type answerPageFn func(context.Context, []int64, repository.PageQuery) (map[int64]*repository.AnswerPage, error)

func (f answerPageFn) Execute(ctx context.Context, ids []int64, q repository.PageQuery) (map[int64]*repository.AnswerPage, error) {
	return f(ctx, ids, q)
}

type replyPageFn func(context.Context, []int64, repository.PageQuery) (map[int64][]*model.Post, error)

func (f replyPageFn) Execute(ctx context.Context, ids []int64, q repository.PageQuery) (map[int64][]*model.Post, error) {
	return f(ctx, ids, q)
}

type anonFn func(context.Context, []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error)

func (f anonFn) Execute(ctx context.Context, keys []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error) {
	return f(ctx, keys)
}

// nopUseCases は「呼ばれたら空を返す」口で全フィールドを埋めた UseCases を返す。
// テストごとに試したいローダーの口だけ差し替える。New はすべてのフィールドを
// 参照するので、埋めずに nil を残すと組み立て時に panic する。
func nopUseCases() UseCases {
	return UseCases{
		GetUsersByIDs:                 sliceFn[*model.User](func(context.Context, []int64) ([]*model.User, error) { return nil, nil }),
		GetPostsByIDs:                 sliceFn[*model.Post](func(context.Context, []int64) ([]*model.Post, error) { return nil, nil }),
		ListMediaByPostIDs:            mapFn[[]*model.Media](func(context.Context, []int64) (map[int64][]*model.Media, error) { return nil, nil }),
		ListMediaByMessageIDs:         mapFn[[]*model.Media](func(context.Context, []int64) (map[int64][]*model.Media, error) { return nil, nil }),
		ListMediaByQuestionIDs:        mapFn[[]*model.Media](func(context.Context, []int64) (map[int64][]*model.Media, error) { return nil, nil }),
		ListMediaByAnswerIDs:          mapFn[[]*model.Media](func(context.Context, []int64) (map[int64][]*model.Media, error) { return nil, nil }),
		GetRepliesByPostIDs:           replyPageFn(func(context.Context, []int64, repository.PageQuery) (map[int64][]*model.Post, error) { return nil, nil }),
		GetRepliesByPostIDsIncludeDel: replyPageFn(func(context.Context, []int64, repository.PageQuery) (map[int64][]*model.Post, error) { return nil, nil }),
		GetFavoritesByPostIDs:         mapFn[[]*model.Favorite](func(context.Context, []int64) (map[int64][]*model.Favorite, error) { return nil, nil }),
		GetMessagesByIDs:              mapFn[*model.Message](func(context.Context, []int64) (map[int64]*model.Message, error) { return nil, nil }),
		ListMentionsByPostIDs:         mapFn[[]*model.Mention](func(context.Context, []int64) (map[int64][]*model.Mention, error) { return nil, nil }),
		ListMentionsByMessageIDs:      mapFn[[]*model.Mention](func(context.Context, []int64) (map[int64][]*model.Mention, error) { return nil, nil }),
		GetRoomsByIDs:                 mapFn[*model.Room](func(context.Context, []int64) (map[int64]*model.Room, error) { return nil, nil }),
		GetQuestionsByIDs:             mapFn[*model.Question](func(context.Context, []int64) (map[int64]*model.Question, error) { return nil, nil }),
		GetAnswersByIDs: mapFn[*repository.AnswerWithLikes](func(context.Context, []int64) (map[int64]*repository.AnswerWithLikes, error) {
			return nil, nil
		}),
		ListAnswerPagesByQuestionIDs: answerPageFn(func(context.Context, []int64, repository.PageQuery) (map[int64]*repository.AnswerPage, error) {
			return nil, nil
		}),
		ListPollOptionResultsByPollIDs: mapFn[[]*repository.PollOptionResult](func(context.Context, []int64) (map[int64][]*repository.PollOptionResult, error) {
			return nil, nil
		}),
		CountPollVotersByPollIDs: mapFn[int](func(context.Context, []int64) (map[int64]int, error) { return nil, nil }),
		GetAnonymousIdentities: anonFn(func(context.Context, []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error) {
			return nil, nil
		}),
	}
}

func ids(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(i + 1)
	}
	return out
}

// TestLoaders_ResolveManyKeysWithOneFetch は「N 件引いてもバッチ関数は1回」を
// ローダーごとに確かめる。ここが2回以上になったら N+1 が戻っている。
func TestLoaders_ResolveManyKeysWithOneFetch(t *testing.T) {
	const n = 25
	want := ids(n)

	tests := []struct {
		name string
		// setup はテスト対象の口を差し替え、その口の呼び出し記録を返す。
		setup func(uc *UseCases) *recorder
		load  func(ctx context.Context, l *Loaders) error
	}{
		{
			name: "UserLoader",
			setup: func(uc *UseCases) *recorder {
				rec := &recorder{}
				uc.GetUsersByIDs = sliceFn[*model.User](func(_ context.Context, keys []int64) ([]*model.User, error) {
					rec.record(keys)
					out := make([]*model.User, 0, len(keys))
					for _, id := range keys {
						out = append(out, &model.User{ID: id})
					}
					return out, nil
				})
				return rec
			},
			load: func(ctx context.Context, l *Loaders) error {
				users, err := l.UserLoader.LoadAll(ctx, want)
				if err != nil {
					return err
				}
				for i, u := range users {
					if u == nil || u.ID != want[i] {
						return errors.New("user came back for the wrong key")
					}
				}
				return nil
			},
		},
		{
			name: "PostLoader",
			setup: func(uc *UseCases) *recorder {
				rec := &recorder{}
				uc.GetPostsByIDs = sliceFn[*model.Post](func(_ context.Context, keys []int64) ([]*model.Post, error) {
					rec.record(keys)
					out := make([]*model.Post, 0, len(keys))
					for _, id := range keys {
						out = append(out, &model.Post{ID: id})
					}
					return out, nil
				})
				return rec
			},
			load: func(ctx context.Context, l *Loaders) error {
				posts, err := l.PostLoader.LoadAll(ctx, want)
				if err != nil {
					return err
				}
				for i, p := range posts {
					if p == nil || p.ID != want[i] {
						return errors.New("post came back for the wrong key")
					}
				}
				return nil
			},
		},
		{
			name: "RoomLoader",
			setup: func(uc *UseCases) *recorder {
				rec := &recorder{}
				uc.GetRoomsByIDs = mapFn[*model.Room](func(_ context.Context, keys []int64) (map[int64]*model.Room, error) {
					rec.record(keys)
					out := make(map[int64]*model.Room, len(keys))
					for _, id := range keys {
						out[id] = &model.Room{ID: id, Type: model.RoomTypeCourse}
					}
					return out, nil
				})
				return rec
			},
			load: func(ctx context.Context, l *Loaders) error {
				rooms, err := l.RoomLoader.LoadAll(ctx, want)
				if err != nil {
					return err
				}
				for i, rm := range rooms {
					if rm == nil || rm.ID != want[i] {
						return errors.New("room came back for the wrong key")
					}
				}
				return nil
			},
		},
		{
			name: "QuestionLoader",
			setup: func(uc *UseCases) *recorder {
				rec := &recorder{}
				uc.GetQuestionsByIDs = mapFn[*model.Question](func(_ context.Context, keys []int64) (map[int64]*model.Question, error) {
					rec.record(keys)
					out := make(map[int64]*model.Question, len(keys))
					for _, id := range keys {
						out[id] = &model.Question{ID: id, RoomID: id * 10}
					}
					return out, nil
				})
				return rec
			},
			load: func(ctx context.Context, l *Loaders) error {
				questions, err := l.QuestionLoader.LoadAll(ctx, want)
				if err != nil {
					return err
				}
				for i, q := range questions {
					if q == nil || q.ID != want[i] {
						return errors.New("question came back for the wrong key")
					}
				}
				return nil
			},
		},
		{
			name: "AnswerLoader",
			setup: func(uc *UseCases) *recorder {
				rec := &recorder{}
				uc.GetAnswersByIDs = mapFn[*repository.AnswerWithLikes](func(_ context.Context, keys []int64) (map[int64]*repository.AnswerWithLikes, error) {
					rec.record(keys)
					out := make(map[int64]*repository.AnswerWithLikes, len(keys))
					for _, id := range keys {
						out[id] = &repository.AnswerWithLikes{Answer: &model.Answer{ID: id}}
					}
					return out, nil
				})
				return rec
			},
			load: func(ctx context.Context, l *Loaders) error {
				answers, err := l.AnswerLoader.LoadAll(ctx, want)
				if err != nil {
					return err
				}
				for i, a := range answers {
					if a == nil || a.Answer.ID != want[i] {
						return errors.New("answer came back for the wrong key")
					}
				}
				return nil
			},
		},
		{
			name: "PollOptionLoader",
			setup: func(uc *UseCases) *recorder {
				rec := &recorder{}
				uc.ListPollOptionResultsByPollIDs = mapFn[[]*repository.PollOptionResult](func(_ context.Context, keys []int64) (map[int64][]*repository.PollOptionResult, error) {
					rec.record(keys)
					out := make(map[int64][]*repository.PollOptionResult, len(keys))
					for _, id := range keys {
						out[id] = []*repository.PollOptionResult{{Option: &model.PollOption{ID: id, PollID: id}}}
					}
					return out, nil
				})
				return rec
			},
			load: func(ctx context.Context, l *Loaders) error {
				results, err := l.PollOptionLoader.LoadAll(ctx, want)
				if err != nil {
					return err
				}
				for i, opts := range results {
					if len(opts) != 1 || opts[0].Option.PollID != want[i] {
						return errors.New("poll options came back for the wrong key")
					}
				}
				return nil
			},
		},
		{
			name: "PollVoterCountLoader",
			setup: func(uc *UseCases) *recorder {
				rec := &recorder{}
				uc.CountPollVotersByPollIDs = mapFn[int](func(_ context.Context, keys []int64) (map[int64]int, error) {
					rec.record(keys)
					out := make(map[int64]int, len(keys))
					for _, id := range keys {
						out[id] = int(id)
					}
					return out, nil
				})
				return rec
			},
			load: func(ctx context.Context, l *Loaders) error {
				counts, err := l.PollVoterCountLoader.LoadAll(ctx, want)
				if err != nil {
					return err
				}
				for i, c := range counts {
					if int64(c) != want[i] {
						return errors.New("voter count came back for the wrong key")
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := nopUseCases()
			rec := tt.setup(&uc)
			loaders := New(uc)

			if err := tt.load(context.Background(), loaders); err != nil {
				t.Fatalf("load: %v", err)
			}
			if got := rec.callCount(); got != 1 {
				t.Fatalf("fetch calls = %d, want 1 (N+1 が戻っている)", got)
			}
			if got := len(rec.lastKeys()); got != n {
				t.Fatalf("keys passed to the batch = %d, want %d (1件ずつ引いている)", got, n)
			}
		})
	}
}

// TestAnonymousIdentityLoader_ResolvesManyKeysWithOneFetch は複合キー
// (roomID, userID) のローダーも1クエリに畳まれることを確かめる。
func TestAnonymousIdentityLoader_ResolvesManyKeysWithOneFetch(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
		got   int
	)
	uc := nopUseCases()
	uc.GetAnonymousIdentities = anonFn(func(_ context.Context, keys []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error) {
		mu.Lock()
		calls++
		got = len(keys)
		mu.Unlock()
		out := make(map[repository.RoomUserKey]*model.RoomAnonymousIdentity, len(keys))
		for _, k := range keys {
			out[k] = &model.RoomAnonymousIdentity{ID: k.UserID, RoomID: k.RoomID, UserID: k.UserID, Label: "匿名001"}
		}
		return out, nil
	})

	keys := make([]repository.RoomUserKey, 0, 20)
	for i := int64(1); i <= 20; i++ {
		keys = append(keys, repository.RoomUserKey{RoomID: 7, UserID: i})
	}

	identities, err := New(uc).AnonymousIdentityLoader.LoadAll(context.Background(), keys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	for i, identity := range identities {
		if identity == nil || identity.UserID != keys[i].UserID {
			t.Fatalf("identity[%d] came back for the wrong key", i)
		}
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
	if got != len(keys) {
		t.Fatalf("keys passed to the batch = %d, want %d", got, len(keys))
	}
}

// TestLoaders_CacheDedupesRepeatedKeys は「同じIDを何度引いてもクエリは1回」を
// 確かめる。Answer.user が回答ごとに同じ質問を引く（項目4）のと、Message が
// userID と user で同じ匿名解決を2回通る（項目7）のは、どちらもこの性質で消える。
func TestLoaders_CacheDedupesRepeatedKeys(t *testing.T) {
	t.Run("QuestionLoader", func(t *testing.T) {
		rec := &recorder{}
		uc := nopUseCases()
		uc.GetQuestionsByIDs = mapFn[*model.Question](func(_ context.Context, keys []int64) (map[int64]*model.Question, error) {
			rec.record(keys)
			return map[int64]*model.Question{42: {ID: 42, RoomID: 7}}, nil
		})
		loaders := New(uc)
		ctx := context.Background()

		// 1件ずつ、間を空けて（＝バッチが閉じたあとで）引き直す。
		for i := 0; i < 5; i++ {
			q, err := loaders.QuestionLoader.Load(ctx, 42)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if q == nil || q.RoomID != 7 {
				t.Fatal("question did not come back")
			}
		}
		if got := rec.callCount(); got != 1 {
			t.Fatalf("fetch calls = %d, want 1 (キャッシュが効いていない)", got)
		}
	})

	t.Run("AnonymousIdentityLoader", func(t *testing.T) {
		var calls int
		uc := nopUseCases()
		uc.GetAnonymousIdentities = anonFn(func(_ context.Context, keys []repository.RoomUserKey) (map[repository.RoomUserKey]*model.RoomAnonymousIdentity, error) {
			calls++
			out := make(map[repository.RoomUserKey]*model.RoomAnonymousIdentity, len(keys))
			for _, k := range keys {
				out[k] = &model.RoomAnonymousIdentity{ID: 9, RoomID: k.RoomID, UserID: k.UserID, Label: "匿名009"}
			}
			return out, nil
		})
		loaders := New(uc)
		key := repository.RoomUserKey{RoomID: 7, UserID: 3}

		first, err := loaders.AnonymousIdentityLoader.Load(context.Background(), key)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		second, err := loaders.AnonymousIdentityLoader.Load(context.Background(), key)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if calls != 1 {
			t.Fatalf("fetch calls = %d, want 1 (userID と user で2回引いている)", calls)
		}
		// 同じ行が返ることまで見る。別々に引くと、途中で採番された場合に
		// userID と user で違う匿名IDが出てしまう。
		if first != second {
			t.Fatal("the same key must resolve to the same identity row")
		}
	})
}

// TestAnswerPageLoader_BatchesPerPageArgs は Question.answers が
// 「質問 N 件 × (一覧 + COUNT)」ではなく、ページ引数ごとに1回で済むことを確かめる。
func TestAnswerPageLoader_BatchesPerPageArgs(t *testing.T) {
	type call struct {
		ids    []int64
		limit  int
		offset int
	}
	var (
		mu    sync.Mutex
		calls []call
	)

	uc := nopUseCases()
	uc.ListAnswerPagesByQuestionIDs = answerPageFn(func(_ context.Context, questionIDs []int64, q repository.PageQuery) (map[int64]*repository.AnswerPage, error) {
		copied := append([]int64(nil), questionIDs...)
		sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
		mu.Lock()
		calls = append(calls, call{ids: copied, limit: q.Limit, offset: q.Offset})
		mu.Unlock()

		out := make(map[int64]*repository.AnswerPage, len(questionIDs))
		for _, qid := range questionIDs {
			out[qid] = &repository.AnswerPage{
				Items: []*repository.AnswerWithLikes{{Answer: &model.Answer{ID: qid, QuestionID: qid}}},
				Total: int(qid),
			}
		}
		return out, nil
	})
	loaders := New(uc)

	keys := []AnswerPageKey{
		{QuestionID: 1, Page: repository.PageQuery{Limit: 20, Offset: 0}},
		{QuestionID: 2, Page: repository.PageQuery{Limit: 20, Offset: 0}},
		{QuestionID: 3, Page: repository.PageQuery{Limit: 20, Offset: 0}},
		// 引数が違うものは別クエリになる（同じ質問でも中身が違うため）。
		{QuestionID: 1, Page: repository.PageQuery{Limit: 5, Offset: 10}},
	}

	pages, err := loaders.AnswerPageLoader.LoadAll(context.Background(), keys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	for i, page := range pages {
		if page == nil || page.Total != int(keys[i].QuestionID) {
			t.Fatalf("page[%d] came back for the wrong key", i)
		}
	}

	if len(calls) != 2 {
		t.Fatalf("fetch calls = %d, want 2 (limit/offset ごとに1回)", len(calls))
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].limit > calls[j].limit })
	if calls[0].limit != 20 || calls[0].offset != 0 || len(calls[0].ids) != 3 {
		t.Fatalf("first group = %+v, want the three questions asked with limit 20 / offset 0", calls[0])
	}
	if calls[1].limit != 5 || calls[1].offset != 10 || len(calls[1].ids) != 1 {
		t.Fatalf("second group = %+v, want the single question asked with limit 5 / offset 10", calls[1])
	}
}

// TestAnswerPageLoader_PropagatesFetchError は失敗を全キーへ配る（部分的に成功した
// 結果を返さない）ことを確かめる。batchFromMap と同じ倒し方。
func TestAnswerPageLoader_PropagatesFetchError(t *testing.T) {
	wantErr := errors.New("boom")
	uc := nopUseCases()
	uc.ListAnswerPagesByQuestionIDs = answerPageFn(func(context.Context, []int64, repository.PageQuery) (map[int64]*repository.AnswerPage, error) {
		return nil, wantErr
	})

	_, err := New(uc).AnswerPageLoader.Load(context.Background(), AnswerPageKey{QuestionID: 1, Page: repository.PageQuery{Limit: 20}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
	}
}

// TestLoaders_MissingKeysResolveToZeroValues は「見つからなかったIDは nil / 0」
// という約束を固定する。バッチ関数が map に入れなかったキーをエラーにしてしまうと、
// 消えた投稿1件で一覧全体が落ちる。
func TestLoaders_MissingKeysResolveToZeroValues(t *testing.T) {
	uc := nopUseCases()
	uc.GetPostsByIDs = sliceFn[*model.Post](func(_ context.Context, keys []int64) ([]*model.Post, error) {
		// 1 だけ返し、2 は返さない（削除済み・ブロック相手の投稿を想定）。
		return []*model.Post{{ID: 1}}, nil
	})
	uc.CountPollVotersByPollIDs = mapFn[int](func(context.Context, []int64) (map[int64]int, error) {
		// 誰も投票していない投票は map に現れない。
		return map[int64]int{}, nil
	})
	loaders := New(uc)
	ctx := context.Background()

	posts, err := loaders.PostLoader.LoadAll(ctx, []int64{1, 2})
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if posts[0] == nil || posts[0].ID != 1 {
		t.Fatal("the post that exists must come back")
	}
	if posts[1] != nil {
		t.Fatal("a missing post must resolve to nil, not an error")
	}

	count, err := loaders.PollVoterCountLoader.Load(ctx, 99)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if count != 0 {
		t.Fatalf("voter count = %d, want 0 for a poll nobody voted on", count)
	}
}

// TestFirstError は LoadAll の束ねられたエラーから1件だけ取り出せることを確かめる。
// そのまま返すと、同じ文言が人数分並んだエラーがクライアントへ出てしまう。
func TestFirstError(t *testing.T) {
	wantErr := errors.New("db is down")

	uc := nopUseCases()
	uc.GetUsersByIDs = sliceFn[*model.User](func(context.Context, []int64) ([]*model.User, error) {
		return nil, wantErr
	})

	_, err := New(uc).UserLoader.LoadAll(context.Background(), []int64{1, 2, 3})
	if err == nil {
		t.Fatal("expected LoadAll to fail")
	}
	if got := FirstError(err); got != wantErr {
		t.Fatalf("FirstError = %v, want the original %v", got, wantErr)
	}
	if got := FirstError(wantErr); got != wantErr {
		t.Fatalf("FirstError must pass a plain error through, got %v", got)
	}
}

// TestFor_PanicsWithoutMiddleware は取り違えたコンテキストで静かに動かないことを確かめる。
func TestFor_PanicsWithoutMiddleware(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("For must panic when the middleware has not run")
		}
	}()
	For(context.Background())
}

// TestWithLoaders_RoundTrips はテスト用の差し込み口が For から読めることを確かめる。
func TestWithLoaders_RoundTrips(t *testing.T) {
	loaders := New(nopUseCases())
	if got := For(WithLoaders(context.Background(), loaders)); got != loaders {
		t.Fatal("For must return the loaders put in by WithLoaders")
	}
}

// TestLoaders_SurfaceTheRealFetchError は、バッチ関数の失敗がそのまま呼び出し側へ
// 届くことを確かめる。値のスライスをキーと同じ長さで返さないと、dataloadgen が
// 「bug in fetch function: 0 values returned for N keys」に差し替えてしまい、
// 本当の原因（DB エラー等）がログにもレスポンスにも残らない。
func TestLoaders_SurfaceTheRealFetchError(t *testing.T) {
	wantErr := errors.New("db is down")

	uc := nopUseCases()
	uc.GetRoomsByIDs = mapFn[*model.Room](func(context.Context, []int64) (map[int64]*model.Room, error) {
		return nil, wantErr
	})
	loaders := New(uc)

	// キーが複数あるバッチでも本当のエラーが返ること（1件のときは元々通っていた）。
	_, err := loaders.RoomLoader.LoadAll(context.Background(), []int64{1, 2, 3})
	if err == nil {
		t.Fatal("expected LoadAll to fail")
	}
	if got := FirstError(err); got != wantErr {
		t.Fatalf("error = %v, want the original %v", got, wantErr)
	}
}
