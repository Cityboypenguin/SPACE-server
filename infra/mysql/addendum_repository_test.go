package mysql

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
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
	for _, f := range []string{"118_create_question_addenda.up.sql", "119_create_answer_addenda.up.sql"} {
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
	t.Setenv("MESSAGE_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	repo, err := NewMySQLAddendumRepository(db)
	if err != nil {
		cleanup()
		t.Fatalf("failed to create the repository: %v", err)
	}
	return repo, cleanup
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
	gone, _ := model.NewAddendum(99, 10, "消された質問へ", now)
	if err := repo.CreateQuestionAddendum(ctx, gone, 2); !errors.Is(err, repository.ErrAddendumTargetNotFound) {
		t.Errorf("err = %v, want ErrAddendumTargetNotFound", err)
	}
	// 上限は質問ごと。別の質問には足せる。
	other, _ := model.NewAddendum(2, 10, "別の質問", now)
	if err := repo.CreateQuestionAddendum(ctx, other, 2); err != nil {
		t.Errorf("another question must have its own limit: %v", err)
	}

	var stored string
	if err := repo.DB.QueryRow(`SELECT body FROM question_addenda ORDER BY id LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "一つ目") {
		t.Error("the body must be encrypted at rest")
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

func TestAddendumRepository_DeleteRemovesOnlyThatAddendum(t *testing.T) {
	repo, cleanup := addendumTestRepo(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Unix(1700000000, 0)

	first, _ := model.NewAddendum(1, 10, "一つ目", now)
	second, _ := model.NewAddendum(1, 10, "二つ目", now)
	for _, a := range []*model.Addendum{first, second} {
		if err := repo.CreateQuestionAddendum(ctx, a, 10); err != nil {
			t.Fatal(err)
		}
	}
	target, found, err := repo.DeleteQuestionAddendum(ctx, first.ID)
	if err != nil || !found || target != 1 {
		t.Fatalf("delete = %d, %v, %v; want question 1", target, found, err)
	}
	if _, found, err := repo.DeleteQuestionAddendum(ctx, first.ID); err != nil || found {
		t.Errorf("second delete: found = %v, err = %v; want not found", found, err)
	}
	got, err := repo.ListQuestionAddenda(ctx, []int64{1})
	if err != nil || len(got[1]) != 1 || got[1][0].Body != "二つ目" {
		t.Errorf("left = %+v (err=%v), want only 二つ目", got[1], err)
	}

	answerAddendum, _ := model.NewAddendum(2, 10, "回答への追記", now)
	if err := repo.CreateAnswerAddendum(ctx, answerAddendum, 10); err != nil {
		t.Fatal(err)
	}
	if target, found, err := repo.DeleteAnswerAddendum(ctx, answerAddendum.ID); err != nil || !found || target != 2 {
		t.Errorf("answer delete = %d, %v, %v; want answer 2", target, found, err)
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
