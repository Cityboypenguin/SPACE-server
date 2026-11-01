package question

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeQuestionRepoForCount struct {
	repository.QuestionRepository
	count  int
	roomID int64
}

func (f *fakeQuestionRepoForCount) CountUnansweredQuestionsByRoomID(_ context.Context, roomID int64) (int, error) {
	f.roomID = roomID
	return f.count, nil
}

func TestCountUnansweredQuestions(t *testing.T) {
	repo := &fakeQuestionRepoForCount{count: 3}
	uc := NewCountUnansweredQuestionsUseCase(repo)

	if _, err := uc.Execute(context.Background(), 5); err == nil {
		t.Fatal("an unauthenticated caller must not read the count")
	}

	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "student"})
	got, err := uc.Execute(ctx, 5)
	if err != nil || got != 3 || repo.roomID != 5 {
		t.Fatalf("Execute = %d, %v (room %d)", got, err, repo.roomID)
	}
}
