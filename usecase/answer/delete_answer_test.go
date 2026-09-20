package answer

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeAnswerRepoForDelete struct {
	repository.AnswerRepository
	answer *model.Answer

	deletedAnswerID int64
}

func (f *fakeAnswerRepoForDelete) GetAnswerByID(_ context.Context, _ int64) (*model.Answer, error) {
	return f.answer, nil
}

func (f *fakeAnswerRepoForDelete) DeleteAnswer(_ context.Context, answerID, _ int64) (bool, error) {
	f.deletedAnswerID = answerID
	return true, nil
}

type deleteAnswerFixture struct {
	answerRepo   *fakeAnswerRepoForDelete
	questionRepo *fakeQuestionRepoForUpdate
	uc           DeleteAnswerUseCase
}

func newDeleteAnswerFixture(writableErr error) *deleteAnswerFixture {
	f := &deleteAnswerFixture{
		answerRepo:   &fakeAnswerRepoForDelete{answer: &model.Answer{ID: 3, QuestionID: 1, AuthorUserID: 7}},
		questionRepo: &fakeQuestionRepoForUpdate{question: &model.Question{ID: 1, RoomID: 5}},
	}
	f.uc = NewDeleteAnswerUseCase(f.questionRepo, f.answerRepo, &fakeRequireWritable{err: writableErr})
	return f
}

func TestDeleteAnswer_RejectsWhenRoomNotWritable(t *testing.T) {
	// 履修をやめた授業や過去の学期では、自分の回答でも削除できない(編集と同じ扱い)。
	f := newDeleteAnswerFixture(errArchivedAnswer)

	if _, err := f.uc.Execute(authorCtx(7), 3); err != errArchivedAnswer {
		t.Fatalf("error = %v, want the writability-check error to be propagated unchanged", err)
	}
	if f.answerRepo.deletedAnswerID != 0 {
		t.Fatal("DeleteAnswer must not be called when the room is not writable")
	}
}

func TestDeleteAnswer_AllowsAuthorWhenWritable(t *testing.T) {
	f := newDeleteAnswerFixture(nil)

	a, err := f.uc.Execute(authorCtx(7), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.answerRepo.deletedAnswerID != 3 {
		t.Fatalf("DeleteAnswer(answerID=%d), want 3", f.answerRepo.deletedAnswerID)
	}
	if a == nil || a.ID != 3 {
		t.Fatal("the deleted answer must be returned so subscribers can be notified")
	}
}

func TestDeleteAnswer_RejectsBestAnswer(t *testing.T) {
	f := newDeleteAnswerFixture(nil)
	bestID := int64(3)
	f.questionRepo.question.BestAnswerID = &bestID

	if _, err := f.uc.Execute(authorCtx(7), 3); err == nil {
		t.Fatal("expected an error when deleting the best answer")
	}
	if f.answerRepo.deletedAnswerID != 0 {
		t.Fatal("DeleteAnswer must not be called for the selected best answer")
	}
}

func TestDeleteAnswer_RejectsOtherUsersAnswer(t *testing.T) {
	f := newDeleteAnswerFixture(nil)

	if _, err := f.uc.Execute(authorCtx(8), 3); err == nil {
		t.Fatal("expected a forbidden error when deleting someone else's answer")
	}
	if f.answerRepo.deletedAnswerID != 0 {
		t.Fatal("DeleteAnswer must not be called for an answer the caller does not own")
	}
}

var errArchivedAnswer = &stubAnswerError{"archived"}

type stubAnswerError struct{ msg string }

func (e *stubAnswerError) Error() string { return e.msg }
