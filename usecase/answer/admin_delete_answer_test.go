package answer

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type adminDeleteAnswerRepo struct {
	repository.AnswerRepository
	answer    *model.Answer
	getErr    error
	deleteErr error
	deletedID int64
	ok        bool
	reads     int
}

func (r *adminDeleteAnswerRepo) GetAnswerByID(context.Context, int64) (*model.Answer, error) {
	r.reads++
	return r.answer, r.getErr
}
func (r *adminDeleteAnswerRepo) AdminDeleteAnswer(_ context.Context, id int64) (bool, error) {
	r.deletedID = id
	return r.ok, r.deleteErr
}

type adminAnswerEvents struct {
	deleted []*model.Answer
	updated []*model.Question
}

func (*adminAnswerEvents) AnswerCreated(context.Context, *model.Answer) {}
func (*adminAnswerEvents) AnswerUpdated(context.Context, *model.Answer) {}
func (e *adminAnswerEvents) AnswerDeleted(_ context.Context, a *model.Answer) {
	e.deleted = append(e.deleted, a)
}
func (*adminAnswerEvents) QuestionCreated(context.Context, *model.Question) {}
func (e *adminAnswerEvents) QuestionUpdated(_ context.Context, q *model.Question) {
	e.updated = append(e.updated, q)
}
func (*adminAnswerEvents) QuestionDeleted(context.Context, *model.Question) {}

func TestAdminDeleteAnswer(t *testing.T) {
	failure := errors.New("database failure")
	for _, tt := range []struct {
		name              string
		ctx               context.Context
		missing           bool
		getErr, deleteErr error
		ok, wantErr       bool
	}{
		{name: "admin deletes another user's answer", ctx: auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"}), ok: true},
		{name: "author cannot use admin deletion", ctx: authorCtx(7), wantErr: true},
		{name: "anonymous cannot delete", ctx: context.Background(), wantErr: true},
		{name: "missing answer", ctx: auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"}), missing: true},
		{name: "concurrent deletion", ctx: auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"})},
		{name: "read failure", ctx: auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"}), getErr: failure, wantErr: true},
		{name: "delete failure", ctx: auth.WithClaims(context.Background(), &auth.Claims{ID: 99, Role: "admin"}), deleteErr: failure, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &adminDeleteAnswerRepo{answer: &model.Answer{ID: 3, QuestionID: 1, AuthorUserID: 7}, ok: tt.ok, getErr: tt.getErr, deleteErr: tt.deleteErr}
			if tt.missing {
				repo.answer = nil
			}
			events := &adminAnswerEvents{}
			q := &model.Question{ID: 1, RoomID: 5}
			uc := NewAdminDeleteAnswerUseCase(events, events, repo, &fakeQuestionRepoForUpdate{question: q})
			ok, err := uc.Execute(tt.ctx, 3)
			if (err != nil) != tt.wantErr || ok != tt.ok {
				t.Fatalf("got (%v, %v), want ok=%v, error=%v", ok, err, tt.ok, tt.wantErr)
			}
			if tt.ok {
				if repo.deletedID != 3 || len(events.deleted) != 1 || events.deleted[0].QuestionID != 1 || len(events.updated) != 1 || events.updated[0] != q {
					t.Fatal("deletion and subscriber updates must target the answer and its question")
				}
			} else if len(events.deleted) != 0 || len(events.updated) != 0 {
				t.Fatal("unsuccessful deletion must not publish events")
			}
			if tt.name == "author cannot use admin deletion" || tt.name == "anonymous cannot delete" {
				if repo.reads != 0 || repo.deletedID != 0 {
					t.Fatal("authorization must precede repository access")
				}
			}
		})
	}
}
