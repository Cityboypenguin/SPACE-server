package question

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeQuestionRepoForAdminDelete struct {
	repository.QuestionRepository
	question *model.Question

	deletedQuestionID int64
}

func (f *fakeQuestionRepoForAdminDelete) GetQuestionByID(_ context.Context, _ int64) (*model.Question, error) {
	return f.question, nil
}

func (f *fakeQuestionRepoForAdminDelete) DeleteQuestion(_ context.Context, questionID int64) (bool, error) {
	f.deletedQuestionID = questionID
	return true, nil
}

// recordingQuestionEvents は配信されたものを覚えるだけの配信先。
type recordingQuestionEvents struct{ deleted []*model.Question }

func (r *recordingQuestionEvents) QuestionCreated(context.Context, *model.Question) {}
func (r *recordingQuestionEvents) QuestionUpdated(context.Context, *model.Question) {}
func (r *recordingQuestionEvents) QuestionDeleted(_ context.Context, q *model.Question) {
	r.deleted = append(r.deleted, q)
}

func adminQuestionCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID, Role: "admin"})
}

func newAdminDeleteFixture() (*fakeQuestionRepoForAdminDelete, *recordingQuestionEvents, DeleteQuestionUseCase) {
	repo := &fakeQuestionRepoForAdminDelete{question: &model.Question{ID: 1, RoomID: 5, AskerUserID: 7}}
	events := &recordingQuestionEvents{}
	return repo, events, NewDeleteQuestionUseCase(events, repo)
}

// 管理者かどうかの判定をユースケースが持つこと。
// 呼び出し側の手順にしておくと、別の入口を足したときにそこだけ抜ける。
func TestDeleteQuestion_RejectsNonAdmins(t *testing.T) {
	repo, events, uc := newAdminDeleteFixture()

	if _, err := uc.Execute(askerCtx(7), 1); err == nil {
		t.Fatal("管理者でない利用者の削除を通してしまった（質問の投稿者であっても）")
	}
	if repo.deletedQuestionID != 0 {
		t.Fatal("認可より先に消してしまった")
	}
	if len(events.deleted) != 0 {
		t.Fatalf("消していないのに配信した: %v", events.deleted)
	}
}

func TestDeleteQuestion_RejectsUnauthenticatedCallers(t *testing.T) {
	repo, _, uc := newAdminDeleteFixture()

	if _, err := uc.Execute(context.Background(), 1); err == nil {
		t.Fatal("未認証の削除を通してしまった")
	}
	if repo.deletedQuestionID != 0 {
		t.Fatal("認可より先に消してしまった")
	}
}

// 管理者が消したら、購読中の画面にも伝わること。
// 配信が無いと、消えた質問が開いたままの画面に残り続ける。
func TestDeleteQuestion_PublishesTheDeletionForSubscribers(t *testing.T) {
	repo, events, uc := newAdminDeleteFixture()

	ok, err := uc.Execute(adminQuestionCtx(99), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || repo.deletedQuestionID != 1 {
		t.Fatalf("ok = %v, deleted = %d", ok, repo.deletedQuestionID)
	}
	if len(events.deleted) != 1 {
		t.Fatalf("配信 = %d 件, want 1 件", len(events.deleted))
	}
	// 配信先は部屋ごとに組み立てるので、どの部屋の質問かが分かる値を渡す必要がある。
	if events.deleted[0].RoomID != 5 {
		t.Fatalf("配信した質問 = %+v, want 部屋つきの質問", events.deleted[0])
	}
}

// 既に無い質問を消そうとしただけなら、失敗にもせず配信もしないこと。
func TestDeleteQuestion_IsQuietWhenTheQuestionIsAlreadyGone(t *testing.T) {
	repo := &fakeQuestionRepoForAdminDelete{}
	events := &recordingQuestionEvents{}
	uc := NewDeleteQuestionUseCase(events, repo)

	ok, err := uc.Execute(adminQuestionCtx(99), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("無い質問の削除を成功として返した")
	}
	if len(events.deleted) != 0 {
		t.Fatalf("消していないのに配信した: %v", events.deleted)
	}
}
