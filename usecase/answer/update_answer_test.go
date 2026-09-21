package answer

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeAnswerRepoForUpdate struct {
	repository.AnswerRepository
	answer      *model.Answer
	updatedBody *string
}

func (f *fakeAnswerRepoForUpdate) GetAnswerByID(_ context.Context, _ int64) (*model.Answer, error) {
	return f.answer, nil
}

func (f *fakeAnswerRepoForUpdate) UpdateAnswerBody(_ context.Context, _, _ int64, body string) (bool, error) {
	f.updatedBody = &body
	return true, nil
}

func (f *fakeAnswerRepoForUpdate) GetAnswerWithLikesByID(_ context.Context, _, _ int64) (*repository.AnswerWithLikes, error) {
	return &repository.AnswerWithLikes{Answer: f.answer}, nil
}

type fakeQuestionRepoForUpdate struct {
	repository.QuestionRepository
	question *model.Question
}

func (f *fakeQuestionRepoForUpdate) GetQuestionByID(_ context.Context, _ int64) (*model.Question, error) {
	return f.question, nil
}

type fakeMediaRepoForUpdate struct {
	repository.MediaRepository
	attached []*model.Media
	deleted  []int64
}

func (f *fakeMediaRepoForUpdate) ListByAnswerIDs(_ context.Context, ids []int64) (map[int64][]*model.Media, error) {
	return map[int64][]*model.Media{ids[0]: f.attached}, nil
}

func (f *fakeMediaRepoForUpdate) DeleteAnswerMedia(_ context.Context, _, mediaID int64) error {
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

type updateAnswerFixture struct {
	answerRepo   *fakeAnswerRepoForUpdate
	questionRepo *fakeQuestionRepoForUpdate
	mediaRepo    *fakeMediaRepoForUpdate
	uc           UpdateAnswerUseCase
}

func newUpdateAnswerFixture(attachedIDs ...int64) *updateAnswerFixture {
	f := &updateAnswerFixture{
		answerRepo:   &fakeAnswerRepoForUpdate{answer: &model.Answer{ID: 3, QuestionID: 1, AuthorUserID: 7}},
		questionRepo: &fakeQuestionRepoForUpdate{question: &model.Question{ID: 1, RoomID: 5}},
		mediaRepo:    &fakeMediaRepoForUpdate{},
	}
	for _, id := range attachedIDs {
		f.mediaRepo.attached = append(f.mediaRepo.attached, &model.Media{ID: id})
	}
	f.uc = NewUpdateAnswerUseCase(nil, f.questionRepo, f.answerRepo, f.mediaRepo, fakeTxManager{}, &fakeRequireWritable{})
	return f
}

func authorCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID})
}

func TestUpdateAnswer_RemovesAttachedMedia(t *testing.T) {
	f := newUpdateAnswerFixture(10, 11)

	if _, err := f.uc.Execute(authorCtx(7), 3, "本文", []int64{11}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.mediaRepo.deleted) != 1 || f.mediaRepo.deleted[0] != 11 {
		t.Fatalf("deleted media = %v, want [11]", f.mediaRepo.deleted)
	}
	if f.answerRepo.updatedBody == nil || *f.answerRepo.updatedBody != "本文" {
		t.Fatal("UpdateAnswerBody was not called with the new body")
	}
}

func TestUpdateAnswer_RejectsMediaNotAttachedToAnswer(t *testing.T) {
	f := newUpdateAnswerFixture(10)

	if _, err := f.uc.Execute(authorCtx(7), 3, "本文", []int64{99}); err == nil {
		t.Fatal("expected error when deleting media that belongs to another answer")
	}
	if len(f.mediaRepo.deleted) != 0 {
		t.Fatalf("no media must be deleted, got %v", f.mediaRepo.deleted)
	}
}

func TestUpdateAnswer_EmptyBodyAllowedWhilePhotoRemains(t *testing.T) {
	f := newUpdateAnswerFixture(10, 11)

	if _, err := f.uc.Execute(authorCtx(7), 3, "", []int64{11}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateAnswer_RejectsEmptyBodyWithoutPhotos(t *testing.T) {
	f := newUpdateAnswerFixture(10)

	if _, err := f.uc.Execute(authorCtx(7), 3, " ", []int64{10}); err == nil {
		t.Fatal("expected error when both the body and every photo would be gone")
	}
	if len(f.mediaRepo.deleted) != 0 {
		t.Fatalf("no media must be deleted, got %v", f.mediaRepo.deleted)
	}
}

func TestUpdateAnswer_RejectsBestAnswer(t *testing.T) {
	f := newUpdateAnswerFixture(10)
	bestID := int64(3)
	f.questionRepo.question.BestAnswerID = &bestID

	if _, err := f.uc.Execute(authorCtx(7), 3, "本文", []int64{10}); err == nil {
		t.Fatal("expected error when editing the best answer")
	}
	if len(f.mediaRepo.deleted) != 0 {
		t.Fatalf("no media must be deleted, got %v", f.mediaRepo.deleted)
	}
}

func TestUpdateAnswer_RejectsOtherUsersAnswer(t *testing.T) {
	f := newUpdateAnswerFixture(10)

	if _, err := f.uc.Execute(authorCtx(8), 3, "本文", []int64{10}); err == nil {
		t.Fatal("expected error when editing someone else's answer")
	}
	if len(f.mediaRepo.deleted) != 0 {
		t.Fatalf("no media must be deleted, got %v", f.mediaRepo.deleted)
	}
}
