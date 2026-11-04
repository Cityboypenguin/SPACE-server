package question

import (
	"context"

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

type fakeMediaRepoForUpdate struct {
	repository.MediaRepository
	attached []*model.Media
}

func (f *fakeMediaRepoForUpdate) ListByQuestionIDs(_ context.Context, ids []int64) (map[int64][]*model.Media, error) {
	return map[int64][]*model.Media{ids[0]: f.attached}, nil
}

type fakeTxManager struct{}

func (fakeTxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fakeRequireWritable struct{ err error }

func (f *fakeRequireWritable) Execute(_ context.Context, _ int64) (*model.Course, error) {
	return nil, f.err
}

func askerCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID})
}
