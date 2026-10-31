package addendum

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeQuestions struct {
	repository.QuestionRepository
	q *model.Question
}

func (f *fakeQuestions) GetQuestionByID(context.Context, int64) (*model.Question, error) {
	return f.q, nil
}

type fakeAnswers struct {
	repository.AnswerRepository
	a *model.Answer
}

func (f *fakeAnswers) GetAnswerByID(context.Context, int64) (*model.Answer, error) { return f.a, nil }
func (f *fakeAnswers) GetAnswerWithLikesByID(_ context.Context, _, _ int64) (*repository.AnswerWithLikes, error) {
	return &repository.AnswerWithLikes{Answer: f.a}, nil
}

type fakeAddenda struct {
	repository.AddendumRepository
	created   []*model.Addendum
	limitHit  bool
	answerers []int64
}

func (f *fakeAddenda) CreateQuestionAddendum(_ context.Context, a *model.Addendum, _ int) error {
	if f.limitHit {
		return repository.ErrAddendumLimit
	}
	f.created = append(f.created, a)
	return nil
}
func (f *fakeAddenda) CreateAnswerAddendum(ctx context.Context, a *model.Addendum, max int) error {
	return f.CreateQuestionAddendum(ctx, a, max)
}
func (f *fakeAddenda) ListAnswererIDs(context.Context, int64) ([]int64, error) {
	return f.answerers, nil
}

type fakeWritable struct{ err error }

func (f *fakeWritable) Execute(context.Context, int64) (*model.Course, error) { return nil, f.err }

type fakeGuard struct {
	block bool
	calls int
}

func (g *fakeGuard) Screen(context.Context, int64, *int64, model.NGWordSource, ...string) error {
	g.calls++
	if g.block {
		return apperr.InvalidInput("blocked")
	}
	return nil
}

type fakeEvents struct{ questionUpdated, answerUpdated int }

func (e *fakeEvents) QuestionCreated(context.Context, *model.Question) {}
func (e *fakeEvents) QuestionUpdated(context.Context, *model.Question) { e.questionUpdated++ }
func (e *fakeEvents) QuestionDeleted(context.Context, *model.Question) {}
func (e *fakeEvents) AnswerCreated(context.Context, *model.Answer)     {}
func (e *fakeEvents) AnswerUpdated(context.Context, *model.Answer)     { e.answerUpdated++ }
func (e *fakeEvents) AnswerDeleted(context.Context, *model.Answer)     {}

type fakeNotifier struct {
	questionTo []int64
	answerFor  *model.Answer
}

func (n *fakeNotifier) QuestionAddendumAdded(_ context.Context, _ *model.Question, ids []int64) {
	n.questionTo = ids
}
func (n *fakeNotifier) AnswerAddendumAdded(_ context.Context, _ *model.Question, a *model.Answer) {
	n.answerFor = a
}

type fixture struct {
	svc      *Service
	addenda  *fakeAddenda
	guard    *fakeGuard
	events   *fakeEvents
	notifier *fakeNotifier
}

const asker, answerer, other = int64(7), int64(8), int64(9)

func newFixture(writableErr error) *fixture {
	f := &fixture{addenda: &fakeAddenda{answerers: []int64{answerer, asker}}, guard: &fakeGuard{}, events: &fakeEvents{}, notifier: &fakeNotifier{}}
	f.svc = NewService(Deps{
		Questions:       &fakeQuestions{q: &model.Question{ID: 1, RoomID: 5, AskerUserID: asker}},
		Answers:         &fakeAnswers{a: &model.Answer{ID: 2, QuestionID: 1, AuthorUserID: answerer}},
		Addenda:         f.addenda,
		RequireWritable: &fakeWritable{err: writableErr},
		NGWords:         f.guard,
		QuestionEvents:  f.events,
		AnswerEvents:    f.events,
		Notifier:        f.notifier,
	})
	return f
}

func ctxAs(id int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: id, Role: "user"})
}

func TestAddToQuestion_OnlyTheAskerCanAdd(t *testing.T) {
	f := newFixture(nil)
	if _, err := f.svc.AddToQuestion(ctxAs(other), 1, "補足"); err == nil || apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Fatalf("err = %v, want Forbidden", err)
	}
	if len(f.addenda.created) != 0 || f.guard.calls != 0 {
		t.Error("a non-asker must not reach the NG check or the save")
	}
}

func TestAddToQuestion_SavesNotifiesAnswerersExceptTheAsker(t *testing.T) {
	f := newFixture(nil)
	if _, err := f.svc.AddToQuestion(ctxAs(asker), 1, "  補足です "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.addenda.created) != 1 || f.addenda.created[0].Body != "補足です" || f.addenda.created[0].TargetID != 1 {
		t.Fatalf("created = %+v", f.addenda.created)
	}
	if len(f.notifier.questionTo) != 1 || f.notifier.questionTo[0] != answerer {
		t.Errorf("notified = %v, want only the answerer (the asker is excluded)", f.notifier.questionTo)
	}
	if f.events.questionUpdated != 1 {
		t.Errorf("questionUpdated = %d, want 1", f.events.questionUpdated)
	}
}

func TestAddToQuestion_RejectsNGWordsEmptyAndTheLimit(t *testing.T) {
	f := newFixture(nil)
	f.guard.block = true
	if _, err := f.svc.AddToQuestion(ctxAs(asker), 1, "NG"); err == nil {
		t.Error("an NG word must be rejected")
	}
	f.guard.block = false
	if _, err := f.svc.AddToQuestion(ctxAs(asker), 1, "   "); err == nil {
		t.Error("an empty addendum must be rejected")
	}
	f.addenda.limitHit = true
	if _, err := f.svc.AddToQuestion(ctxAs(asker), 1, "11件目"); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidInput {
		t.Errorf("err = %v, want InvalidInput for the limit", err)
	}
	if f.notifier.questionTo != nil || f.events.questionUpdated != 0 {
		t.Error("nothing may be notified or broadcast when the save fails")
	}
}

func TestAddToQuestion_NotWritableRoom(t *testing.T) {
	f := newFixture(errors.New("not writable"))
	if _, err := f.svc.AddToQuestion(ctxAs(asker), 1, "補足"); err == nil {
		t.Fatal("a closed room must reject")
	}
	if len(f.addenda.created) != 0 {
		t.Error("nothing may be saved")
	}
}

func TestAddToAnswer_OnlyTheAnswererCanAddAndTheAskerIsNotified(t *testing.T) {
	f := newFixture(nil)
	if _, err := f.svc.AddToAnswer(ctxAs(asker), 2, "補足"); err == nil || apperr.CodeOf(err) != apperr.CodeForbidden {
		t.Fatalf("err = %v, want Forbidden for a non-author", err)
	}
	if _, err := f.svc.AddToAnswer(ctxAs(answerer), 2, "補足です"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.notifier.answerFor == nil || f.notifier.answerFor.ID != 2 {
		t.Error("the asker must be notified about the answer's addendum")
	}
	if f.events.answerUpdated != 1 {
		t.Errorf("answerUpdated = %d, want 1", f.events.answerUpdated)
	}
}
