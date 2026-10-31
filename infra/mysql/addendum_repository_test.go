package mysql

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// question_addenda / answer_addenda は実際のマイグレーションの SQL をそのまま流す。
// 参照先の questions / answers / users は、外部キーが張れる最小の形。
func addendumTestRepo(t *testing.T) (*MySQLAddendumRepository, func()) {
	t.Helper()
	db, cleanup := unreadTestDB(t)
	mustExec(t, db, `CREATE TABLE users (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))`)
	mustExec(t, db, `CREATE TABLE questions (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id))`)
	mustExec(t, db, `CREATE TABLE answers (id BIGINT NOT NULL AUTO_INCREMENT, question_id BIGINT NOT NULL, author_user_id BIGINT NOT NULL, PRIMARY KEY (id))`)
	for _, f := range []string{"080_create_question_addenda.up.sql", "081_create_answer_addenda.up.sql"} {
		b, err := os.ReadFile("../../db/migrations/" + f)
		if err != nil {
			cleanup()
			t.Fatalf("failed to read the migration %s: %v", f, err)
		}
		mustExec(t, db, string(b))
	}
	mustExec(t, db, `INSERT INTO users (id) VALUES (10), (11), (12)`)
	mustExec(t, db, `INSERT INTO questions (id) VALUES (1), (2)`)
	mustExec(t, db, `INSERT INTO answers (id, question_id, author_user_id) VALUES (1, 1, 11), (2, 1, 12), (3, 1, 11), (4, 2, 12)`)
	return &MySQLAddendumRepository{DB: db}, cleanup
}

func TestAddendumRepository_CreateListAndLimit(t *testing.T) {
	repo, cleanup := addendumTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Unix(1700000000, 0)

	for _, body := range []string{"一つ目", "二つ目"} {
		a, err := model.NewAddendum(1, 10, body, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateQuestionAddendum(ctx, a, 2); err != nil {
			t.Fatalf("create %s: %v", body, err)
		}
		if a.ID == 0 {
			t.Error("the new ID must be set")
		}
	}
	third, _ := model.NewAddendum(1, 10, "三つ目", now)
	if err := repo.CreateQuestionAddendum(ctx, third, 2); !errors.Is(err, repository.ErrAddendumLimit) {
		t.Errorf("err = %v, want ErrAddendumLimit", err)
	}
	// 上限は質問ごと。別の質問には足せる。
	other, _ := model.NewAddendum(2, 10, "別の質問", now)
	if err := repo.CreateQuestionAddendum(ctx, other, 2); err != nil {
		t.Errorf("another question must have its own limit: %v", err)
	}

	got, err := repo.ListQuestionAddenda(ctx, []int64{1, 2, 99})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[1]) != 2 || got[1][0].Body != "一つ目" || got[1][1].Body != "二つ目" {
		t.Errorf("addenda of question 1 = %+v, want 2 in order", got[1])
	}
	if len(got[2]) != 1 || len(got[99]) != 0 {
		t.Errorf("got[2]=%d got[99]=%d", len(got[2]), len(got[99]))
	}

	ansA, _ := model.NewAddendum(1, 11, "回答への追記", now)
	if err := repo.CreateAnswerAddendum(ctx, ansA, 2); err != nil {
		t.Fatal(err)
	}
	gotA, err := repo.ListAnswerAddenda(ctx, []int64{1, 2})
	if err != nil || len(gotA[1]) != 1 || len(gotA[2]) != 0 {
		t.Errorf("answer addenda = %+v (err=%v)", gotA, err)
	}
}

func TestAddendumRepository_ListAnswererIDsIsDistinct(t *testing.T) {
	repo, cleanup := addendumTestRepo(t)
	defer cleanup()
	ids, err := repo.ListAnswererIDs(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != 11 || ids[1] != 12 {
		t.Errorf("ids = %v, want [11 12] (重複なし)", ids)
	}
}

// 同時に追記しても、上限を超えない。
func TestAddendumRepository_ConcurrentCreatesRespectTheLimit(t *testing.T) {
	repo, cleanup := addendumTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Unix(1700000000, 0)
	const limit, attempts = 3, 12

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _ := model.NewAddendum(1, 10, "同時", now)
			err := repo.CreateQuestionAddendum(ctx, a, limit)
			if err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			} else if !errors.Is(err, repository.ErrAddendumLimit) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	got, err := repo.ListQuestionAddenda(ctx, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if succeeded != limit || len(got[1]) != limit {
		t.Errorf("succeeded = %d, stored = %d, want exactly %d", succeeded, len(got[1]), limit)
	}
}
