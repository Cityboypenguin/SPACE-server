package mysql

import (
	"context"
	"testing"
)

func TestAdminDeleteAnswer_PreservesQuestionAndOtherAnswers(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "admin_delete_answer", []string{
		`CREATE TABLE questions (id BIGINT PRIMARY KEY, best_answer_id BIGINT NULL, is_answered BOOLEAN NOT NULL, updated_at BIGINT NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE answers (id BIGINT PRIMARY KEY, question_id BIGINT NOT NULL, FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE) ENGINE=InnoDB`,
		`ALTER TABLE questions ADD FOREIGN KEY (best_answer_id) REFERENCES answers(id) ON DELETE SET NULL`,
		`CREATE TABLE answer_likes (id BIGINT PRIMARY KEY, answer_id BIGINT NOT NULL, FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE CASCADE) ENGINE=InnoDB`,
		`CREATE TABLE answer_media (id BIGINT PRIMARY KEY, answer_id BIGINT NOT NULL, FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE CASCADE) ENGINE=InnoDB`,
		`CREATE TABLE answer_addenda (id BIGINT PRIMARY KEY, answer_id BIGINT NOT NULL, FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE CASCADE) ENGINE=InnoDB`,
	})
	defer cleanup()
	for _, stmt := range []string{
		`INSERT INTO questions VALUES (1,NULL,FALSE,0)`,
		`INSERT INTO answers VALUES (3,1),(4,1),(5,1)`,
		`UPDATE questions SET best_answer_id=3,is_answered=TRUE WHERE id=1`,
		`INSERT INTO answer_likes VALUES (1,3),(2,4)`,
		`INSERT INTO answer_media VALUES (1,3),(2,4)`,
		`INSERT INTO answer_addenda VALUES (1,3),(2,4)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	repo := &MySQLAnswerRepository{DB: db}
	ctx := context.Background()
	// Deleting a different answer must preserve the selected best answer.
	if ok, err := repo.AdminDeleteAnswer(ctx, 5); err != nil || !ok {
		t.Fatalf("delete ordinary answer: %v, %v", ok, err)
	}
	var bestID int64
	var resolved bool
	if err := db.QueryRow(`SELECT best_answer_id,is_answered FROM questions WHERE id=1`).Scan(&bestID, &resolved); err != nil || bestID != 3 || !resolved {
		t.Fatalf("best answer changed: %d, %v, %v", bestID, resolved, err)
	}
	if ok, err := repo.AdminDeleteAnswer(ctx, 3); err != nil || !ok {
		t.Fatalf("delete best answer: %v, %v", ok, err)
	}
	for _, table := range []string{"questions", "answers", "answer_likes", "answer_media", "answer_addenda"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s rows=%d, err=%v; want only target answer and its dependents removed", table, count, err)
		}
	}
	var cleared bool
	if err := db.QueryRow(`SELECT best_answer_id IS NULL,is_answered FROM questions WHERE id=1`).Scan(&cleared, &resolved); err != nil || !cleared || resolved {
		t.Fatalf("question must become unresolved: %v, %v, %v", cleared, resolved, err)
	}
	if ok, err := repo.AdminDeleteAnswer(ctx, 3); err != nil || ok {
		t.Fatalf("repeated deletion: %v, %v", ok, err)
	}

	// A failed DELETE must roll back the preceding best-answer reset.
	for _, stmt := range []string{
		`UPDATE questions SET best_answer_id=4,is_answered=TRUE WHERE id=1`,
		`CREATE TABLE deletion_blocker (answer_id BIGINT NOT NULL, FOREIGN KEY (answer_id) REFERENCES answers(id) ON DELETE RESTRICT) ENGINE=InnoDB`,
		`INSERT INTO deletion_blocker VALUES (4)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.AdminDeleteAnswer(ctx, 4); err == nil {
		t.Fatal("expected restricted deletion to fail")
	}
	if err := db.QueryRow(`SELECT best_answer_id,is_answered FROM questions WHERE id=1`).Scan(&bestID, &resolved); err != nil || bestID != 4 || !resolved {
		t.Fatalf("failed deletion must preserve best answer: %d, %v, %v", bestID, resolved, err)
	}
}
