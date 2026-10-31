package ngword

import (
	"context"
	"errors"
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
}

func (r *fakeRepo) ListNGWords(context.Context) ([]*model.NGWord, error) {
	r.lists++
	return r.words, r.err
}
func (r *fakeRepo) CreateNGWord(_ context.Context, w *model.NGWord) error {
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
	if got := err.Error(); got != "" && contains(got, "Respon") {
		t.Errorf("the message must not reveal the word: %q", got)
	}

	if err := g.Screen(context.Background(), 10, &room, model.NGWordSourceMessage, "今日の課題は？"); err != nil {
		t.Errorf("clean text blocked: %v", err)
	}
	if len(repo.hits) != 1 {
		t.Errorf("a clean text must not be recorded")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
