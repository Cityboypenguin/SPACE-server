package mysql

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ng_words / ng_word_hits は実際のマイグレーションの SQL をそのまま流す
// （ユニーク制約・外部キーの効き方まで確かめるため）。users だけは集計が読む列だけの簡易版。
func ngWordTestRepo(t *testing.T) (*MySQLNGWordRepository, func()) {
	t.Helper()
	db, cleanup := unreadTestDB(t)
	mustExec(t, db, `CREATE TABLE users (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))`)
	mustExec(t, db, `CREATE TABLE user_accounts (user_id BIGINT NOT NULL, account_id VARCHAR(255) NOT NULL, name VARCHAR(255) NOT NULL, PRIMARY KEY (user_id))`)
	for _, f := range []string{"116_create_ng_words.up.sql", "117_create_ng_word_hits.up.sql"} {
		b, err := os.ReadFile("../../db/migrations/" + f)
		if err != nil {
			cleanup()
			t.Fatalf("failed to read the migration %s: %v", f, err)
		}
		mustExec(t, db, string(b))
	}
	return &MySQLNGWordRepository{DB: db}, cleanup
}

func newTestNGWord(t *testing.T, pattern string) *model.NGWord {
	t.Helper()
	w, err := model.NewNGWord(pattern, model.NGWordMatchWord, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestNGWordRepository_LimitAndDuplicate(t *testing.T) {
	repo, cleanup := ngWordTestRepo(t)
	defer cleanup()
	ctx := context.Background()

	for _, p := range []string{"a1", "a2"} {
		if err := repo.CreateNGWord(ctx, newTestNGWord(t, p), 2); err != nil {
			t.Fatalf("create %s: %v", p, err)
		}
	}
	if err := repo.CreateNGWord(ctx, newTestNGWord(t, "a3"), 2); !errors.Is(err, repository.ErrNGWordLimit) {
		t.Errorf("err = %v, want ErrNGWordLimit (上限に達したら登録しない)", err)
	}
	if err := repo.CreateNGWord(ctx, newTestNGWord(t, "a1"), 10); !errors.Is(err, repository.ErrDuplicateKey) {
		t.Errorf("err = %v, want ErrDuplicateKey", err)
	}
	words, err := repo.ListNGWords(ctx)
	if err != nil || len(words) != 2 {
		t.Fatalf("words = %d (err=%v), want 2", len(words), err)
	}
}

func TestNGWordRepository_HitStatsSurviveDeletingTheWord(t *testing.T) {
	repo, cleanup := ngWordTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	mustExec(t, repo.DB, `INSERT INTO users (id) VALUES (10), (11)`)
	mustExec(t, repo.DB, `INSERT INTO user_accounts (user_id, account_id, name) VALUES (10, 'alice', 'Alice'), (11, 'bob', 'Bob')`)

	w := newTestNGWord(t, "respon")
	if err := repo.CreateNGWord(ctx, w, 10); err != nil {
		t.Fatal(err)
	}
	room := int64(1)
	now := time.Unix(1700000000, 0)
	for i, uid := range []int64{10, 10, 11} {
		src := model.NGWordSourceMessage
		if i == 2 {
			src = model.NGWordSourceAnswer
		}
		if err := repo.RecordNGWordHit(ctx, &model.NGWordHit{NGWordID: &w.ID, Pattern: w.Pattern, UserID: uid, RoomID: &room, Source: src, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}

	check := func(label string) {
		st, err := repo.NGWordHitStats(ctx, 10)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if st.Total != 3 || len(st.ByWord) != 1 || st.ByWord[0].Pattern != "respon" || st.ByWord[0].Count != 3 {
			t.Errorf("%s: by word = %+v total=%d", label, st.ByWord, st.Total)
		}
		if len(st.ByUser) != 2 || st.ByUser[0].AccountID != "alice" || st.ByUser[0].Count != 2 || st.ByUser[1].AccountID != "bob" || st.ByUser[1].Count != 1 {
			t.Errorf("%s: by user = %+v", label, st.ByUser)
		}
	}
	check("登録中")

	if deleted, err := repo.DeleteNGWord(ctx, w.ID); err != nil || !deleted {
		t.Fatalf("delete = %v (err=%v)", deleted, err)
	}
	check("NGワード削除後も回数は残る")

	if st, _ := repo.NGWordHitStats(ctx, 1); len(st.ByUser) != 1 {
		t.Errorf("topN must limit the list, got %d", len(st.ByUser))
	}
}
