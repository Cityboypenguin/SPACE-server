package ngword

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeRepo struct {
	words []*model.NGWord
	hits  []*model.NGWordHit
	lists int
	err   error
	// onList は読み込みの途中で起きることの代役。
	onList func()
}

func (r *fakeRepo) ListNGWords(context.Context) ([]*model.NGWord, error) {
	r.lists++
	words := r.words
	if r.onList != nil {
		r.onList()
	}
	return words, r.err
}
func (r *fakeRepo) CreateNGWord(_ context.Context, w *model.NGWord, _ int) error {
	for _, e := range r.words {
		if e.Pattern == w.Pattern && e.MatchType == w.MatchType {
			return repository.ErrDuplicateKey
		}
	}
	w.ID = int64(len(r.words) + 1)
	r.words = append(r.words, w)
	return nil
}
func (r *fakeRepo) DeleteNGWord(context.Context, int64) (bool, error) { return true, nil }
func (r *fakeRepo) RecordNGWordHit(_ context.Context, h *model.NGWordHit) error {
	r.hits = append(r.hits, h)
	return nil
}
func (r *fakeRepo) NGWordHitStats(context.Context, int) (*model.NGWordHitStats, error) {
	return &model.NGWordHitStats{}, nil
}

func TestGuard_BlocksAndRecords(t *testing.T) {
	repo := &fakeRepo{words: []*model.NGWord{{ID: 7, Pattern: "Respon", MatchType: model.NGWordMatchWord}}}
	g := NewGuard(repo)
	room := int64(3)

	err := g.Screen(context.Background(), 10, &room, model.NGWordSourceMessage, "レスポン respon です")
	if err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Fatalf("err = %v, want InvalidInput", err)
	}
	if len(repo.hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(repo.hits))
	}
	h := repo.hits[0]
	if h.UserID != 10 || h.Pattern != "Respon" || h.Source != model.NGWordSourceMessage || h.NGWordID == nil || *h.NGWordID != 7 || h.RoomID == nil || *h.RoomID != 3 {
		t.Errorf("recorded hit = %+v", h)
	}
	// どの言葉に当たったかを、ユーザー向けの文言に出さない。
	if got := err.Error(); strings.Contains(got, "Respon") {
		t.Errorf("the message must not reveal the word: %q", got)
	}

	if err := g.Screen(context.Background(), 10, &room, model.NGWordSourceMessage, "今日の課題は？"); err != nil {
		t.Errorf("clean text blocked: %v", err)
	}
	if len(repo.hits) != 1 {
		t.Errorf("a clean text must not be recorded")
	}
}

func TestGuard_FailsOpenWhenTheListCannotBeLoaded(t *testing.T) {
	g := NewGuard(&fakeRepo{err: errors.New("db down")})
	if err := g.Screen(context.Background(), 1, nil, model.NGWordSourceAnswer, "Respon"); err != nil {
		t.Errorf("err = %v, want nil (検査の不具合で投稿を止めない)", err)
	}
}

func TestGuard_CachesAndInvalidates(t *testing.T) {
	repo := &fakeRepo{words: []*model.NGWord{{ID: 1, Pattern: "abc", MatchType: model.NGWordMatchWord}}}
	g := NewGuard(repo).(*guard)
	now := time.Unix(1700000000, 0)
	g.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		_ = g.Screen(context.Background(), 1, nil, model.NGWordSourceMessage, "x")
	}
	if repo.lists != 1 {
		t.Errorf("lists = %d, want 1 (キャッシュ中は引き直さない)", repo.lists)
	}
	g.Invalidate()
	_ = g.Screen(context.Background(), 1, nil, model.NGWordSourceMessage, "x")
	if repo.lists != 2 {
		t.Errorf("lists = %d, want 2 (変更後は引き直す)", repo.lists)
	}
	now = now.Add(cacheTTL + time.Second)
	_ = g.Screen(context.Background(), 1, nil, model.NGWordSourceMessage, "x")
	if repo.lists != 3 {
		t.Errorf("lists = %d, want 3 (TTL を過ぎたら引き直す)", repo.lists)
	}
}

// 読み込みの途中で登録が変わったら、古い内容を覚えない。ロックを持ったまま読むと、
// ここの Invalidate が止まって、テストごと固まる。
func TestGuard_DoesNotCacheAListThatChangedWhileLoading(t *testing.T) {
	repo := &fakeRepo{words: []*model.NGWord{{ID: 1, Pattern: "abc", MatchType: model.NGWordMatchWord}}}
	g := NewGuard(repo)
	repo.onList = func() {
		repo.onList = nil
		g.Invalidate()
	}

	_ = g.Screen(context.Background(), 1, nil, model.NGWordSourceMessage, "x")
	_ = g.Screen(context.Background(), 1, nil, model.NGWordSourceMessage, "x")
	if repo.lists != 2 {
		t.Errorf("lists = %d, want 2 (読み込み中に変わった内容は覚えない)", repo.lists)
	}
}

func TestGuard_KeepsThePreviousListWhenReloadFails(t *testing.T) {
	repo := &fakeRepo{words: []*model.NGWord{{ID: 1, Pattern: "abc", MatchType: model.NGWordMatchWord}}}
	g := NewGuard(repo).(*guard)
	now := time.Unix(1700000000, 0)
	g.now = func() time.Time { return now }
	if err := g.Screen(context.Background(), 1, nil, model.NGWordSourceMessage, "x"); err != nil {
		t.Fatal(err)
	}

	now = now.Add(cacheTTL + time.Second)
	repo.err = errors.New("db down")
	if err := g.Screen(context.Background(), 1, nil, model.NGWordSourceMessage, "abc"); err == nil {
		t.Error("the previous list must still block the word while the reload fails")
	}
}

func TestGuardReturnsAllMaskedFields(t *testing.T) {
	repo := &fakeRepo{words: []*model.NGWord{{ID: 1, Pattern: "respon", MatchType: model.NGWordMatchWord}}}
	err := NewGuard(repo).Screen(context.Background(), 1, nil, model.NGWordSourcePoll, "前RESPON後", "安全な選択肢", "選択肢respon")
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("want BlockedError, got %v", err)
	}
	if len(blocked.Texts) != 2 || blocked.Texts[0].Original != "前RESPON後" || blocked.Texts[0].Masked != "前××××後" || blocked.Texts[1].Masked != "選択肢××××" {
		t.Fatalf("masked texts = %+v", blocked.Texts)
	}
	if len(repo.hits) != 1 {
		t.Fatalf("want one blocked attempt, got %d", len(repo.hits))
	}
}
