package question

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeQuestionRepoForDelete struct {
	repository.QuestionRepository
	question *model.Question

	deletedQuestionID int64
}

func (f *fakeQuestionRepoForDelete) GetQuestionByID(_ context.Context, _ int64) (*model.Question, error) {
	return f.question, nil
}

func (f *fakeQuestionRepoForDelete) DeleteQuestionByAsker(_ context.Context, questionID, _ int64) (bool, error) {
	f.deletedQuestionID = questionID
	return true, nil
}

func newDeleteMyQuestionFixture(writableErr error) (*fakeQuestionRepoForDelete, DeleteMyQuestionUseCase) {
	repo := &fakeQuestionRepoForDelete{question: &model.Question{ID: 1, RoomID: 5, AskerUserID: 7}}
	return repo, NewDeleteMyQuestionUseCase(repo, &fakeRequireWritable{err: writableErr})
}

func TestDeleteMyQuestion_RejectsWhenRoomNotWritable(t *testing.T) {
	// 履修をやめた授業や過去の学期では、自分の質問でも削除できない(編集と同じ扱い)。
	repo, uc := newDeleteMyQuestionFixture(errArchivedQuestion)

	if _, err := uc.Execute(askerCtx(7), 1); err != errArchivedQuestion {
		t.Fatalf("error = %v, want the writability-check error to be propagated unchanged", err)
	}
	if repo.deletedQuestionID != 0 {
		t.Fatal("DeleteQuestionByAsker must not be called when the room is not writable")
	}
}

func TestDeleteMyQuestion_AllowsAskerWhenWritable(t *testing.T) {
	repo, uc := newDeleteMyQuestionFixture(nil)

	q, err := uc.Execute(askerCtx(7), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deletedQuestionID != 1 {
		t.Fatalf("DeleteQuestionByAsker(questionID=%d), want 1", repo.deletedQuestionID)
	}
	if q == nil || q.ID != 1 {
		t.Fatal("the deleted question must be returned so subscribers can be notified")
	}
}

func TestDeleteMyQuestion_RejectsOtherUsersQuestion(t *testing.T) {
	repo, uc := newDeleteMyQuestionFixture(nil)

	if _, err := uc.Execute(askerCtx(8), 1); err == nil {
		t.Fatal("expected a forbidden error when deleting someone else's question")
	}
	if repo.deletedQuestionID != 0 {
		t.Fatal("DeleteQuestionByAsker must not be called for a question the caller did not ask")
	}
}

var errArchivedQuestion = &stubQuestionError{"archived"}

type stubQuestionError struct{ msg string }

func (e *stubQuestionError) Error() string { return e.msg }
