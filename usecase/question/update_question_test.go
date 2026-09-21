package question

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeQuestionRepoForUpdate struct {
	repository.QuestionRepository
	question    *model.Question
	updatedBody *string
}

func (f *fakeQuestionRepoForUpdate) GetQuestionByID(_ context.Context, _ int64) (*model.Question, error) {
	return f.question, nil
}

func (f *fakeQuestionRepoForUpdate) UpdateQuestionBody(_ context.Context, _, _ int64, body string) (bool, error) {
	f.updatedBody = &body
	return true, nil
}

type fakeMediaRepoForUpdate struct {
	repository.MediaRepository
	attached []*model.Media
	deleted  []int64
}

func (f *fakeMediaRepoForUpdate) ListByQuestionIDs(_ context.Context, ids []int64) (map[int64][]*model.Media, error) {
	return map[int64][]*model.Media{ids[0]: f.attached}, nil
}

func (f *fakeMediaRepoForUpdate) DeleteQuestionMedia(_ context.Context, _, mediaID int64) error {
	f.deleted = append(f.deleted, mediaID)
	return nil
}

type fakeTxManager struct{}

func (fakeTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fakeRequireWritable struct{ err error }

func (f *fakeRequireWritable) Execute(_ context.Context, _ int64) (*model.Course, error) {
	return nil, f.err
}

func newUpdateQuestionFixture(attachedIDs ...int64) (*fakeQuestionRepoForUpdate, *fakeMediaRepoForUpdate, UpdateQuestionUseCase) {
	questionRepo := &fakeQuestionRepoForUpdate{question: &model.Question{ID: 1, RoomID: 5, AskerUserID: 7}}
	mediaRepo := &fakeMediaRepoForUpdate{}
	for _, id := range attachedIDs {
		mediaRepo.attached = append(mediaRepo.attached, &model.Media{ID: id})
	}
	uc := NewUpdateQuestionUseCase(nil, questionRepo, mediaRepo, fakeTxManager{}, &fakeRequireWritable{})
	return questionRepo, mediaRepo, uc
}

func askerCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID})
}

func TestUpdateQuestion_RemovesAttachedMedia(t *testing.T) {
	questionRepo, mediaRepo, uc := newUpdateQuestionFixture(10, 11)

	if _, err := uc.Execute(askerCtx(7), 1, "本文", []int64{11}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mediaRepo.deleted) != 1 || mediaRepo.deleted[0] != 11 {
		t.Fatalf("deleted media = %v, want [11]", mediaRepo.deleted)
	}
	if questionRepo.updatedBody == nil || *questionRepo.updatedBody != "本文" {
		t.Fatal("UpdateQuestionBody was not called with the new body")
	}
}

func TestUpdateQuestion_RejectsMediaNotAttachedToQuestion(t *testing.T) {
	_, mediaRepo, uc := newUpdateQuestionFixture(10)

	if _, err := uc.Execute(askerCtx(7), 1, "本文", []int64{99}); err == nil {
		t.Fatal("expected error when deleting media that belongs to another question")
	}
	if len(mediaRepo.deleted) != 0 {
		t.Fatalf("no media must be deleted, got %v", mediaRepo.deleted)
	}
}

func TestUpdateQuestion_EmptyBodyAllowedWhilePhotoRemains(t *testing.T) {
	_, _, uc := newUpdateQuestionFixture(10, 11)

	if _, err := uc.Execute(askerCtx(7), 1, "", []int64{11}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateQuestion_RejectsEmptyBodyWithoutPhotos(t *testing.T) {
	_, mediaRepo, uc := newUpdateQuestionFixture(10)

	if _, err := uc.Execute(askerCtx(7), 1, "  ", []int64{10}); err == nil {
		t.Fatal("expected error when both the body and every photo would be gone")
	}
	if len(mediaRepo.deleted) != 0 {
		t.Fatalf("no media must be deleted, got %v", mediaRepo.deleted)
	}
}

func TestUpdateQuestion_RejectsOtherUsersQuestion(t *testing.T) {
	_, mediaRepo, uc := newUpdateQuestionFixture(10)

	if _, err := uc.Execute(askerCtx(8), 1, "本文", []int64{10}); err == nil {
		t.Fatal("expected error when editing someone else's question")
	}
	if len(mediaRepo.deleted) != 0 {
		t.Fatalf("no media must be deleted, got %v", mediaRepo.deleted)
	}
}
