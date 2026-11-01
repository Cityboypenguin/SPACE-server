package mysql

import (
	"context"
	"testing"
)

// 件数の条件は SQL そのものなので、実物の MySQL で確かめる。
func TestCountUnansweredQuestionsByRoomID(t *testing.T) {
	db, cleanup := throwawaySchemaDB(t, "space_question_count_test", []string{
		`CREATE TABLE questions (
			id BIGINT NOT NULL AUTO_INCREMENT,
			room_id BIGINT NOT NULL,
			is_answered BOOLEAN NOT NULL DEFAULT FALSE,
			best_answer_id BIGINT NULL,
			PRIMARY KEY (id)
		)`,
		`INSERT INTO questions (room_id, is_answered, best_answer_id) VALUES
			(1, FALSE, NULL),
			(1, FALSE, NULL),
			(1, TRUE, 10),
			(1, TRUE, NULL),
			(2, FALSE, NULL)`,
	})
	defer cleanup()

	repo := &MySQLQuestionRepository{DB: db}
	// is_answered が残っていても、ベストアンサーの回答が消えた質問は未決として数える。
	got, err := repo.CountUnansweredQuestionsByRoomID(context.Background(), 1)
	if err != nil {
		t.Fatalf("CountUnansweredQuestionsByRoomID: %v", err)
	}
	if got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
}
