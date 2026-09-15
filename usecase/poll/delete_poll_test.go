package poll

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakePollRepoForDelete struct {
	repository.PollRepository
	poll *model.Poll

	deletedPollID int64
}

func (f *fakePollRepoForDelete) GetPollByID(_ context.Context, _ int64) (*model.Poll, error) {
	return f.poll, nil
}

func (f *fakePollRepoForDelete) DeletePoll(_ context.Context, pollID int64) (bool, error) {
	f.deletedPollID = pollID
	return true, nil
}

func adminCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID, Role: "admin"})
}

func TestDeletePoll_RejectsWhenRoomNotWritable(t *testing.T) {
	// 履修をやめた授業や過去の学期では、自分が作った投票でも削除できない
	// (作成・投票と同じ扱い)。
	repo := &fakePollRepoForDelete{poll: &model.Poll{ID: 1, RoomID: 5, AuthorUserID: 7}}
	uc := NewDeletePollUseCase(repo, &fakeRequireWritable{err: errArchived})

	if _, err := uc.Execute(authedCtx(7), 1); err != errArchived {
		t.Fatalf("error = %v, want the writability-check error to be propagated unchanged", err)
	}
	if repo.deletedPollID != 0 {
		t.Fatal("DeletePoll must not be called when the room is not writable")
	}
}

func TestDeletePoll_AllowsAuthorWhenWritable(t *testing.T) {
	repo := &fakePollRepoForDelete{poll: &model.Poll{ID: 1, RoomID: 5, AuthorUserID: 7}}
	uc := NewDeletePollUseCase(repo, &fakeRequireWritable{})

	p, err := uc.Execute(authedCtx(7), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deletedPollID != 1 {
		t.Fatalf("DeletePoll(pollID=%d), want 1", repo.deletedPollID)
	}
	if p == nil || p.ID != 1 {
		t.Fatal("the deleted poll must be returned so subscribers can be notified")
	}
}

func TestDeletePoll_AdminDeletesEvenWhenRoomNotWritable(t *testing.T) {
	// 管理者のモデレーションは学期や履修に縛られない。
	repo := &fakePollRepoForDelete{poll: &model.Poll{ID: 1, RoomID: 5, AuthorUserID: 7}}
	uc := NewDeletePollUseCase(repo, &fakeRequireWritable{err: errArchived})

	if _, err := uc.Execute(adminCtx(99), 1); err != nil {
		t.Fatalf("unexpected error for an administrator: %v", err)
	}
	if repo.deletedPollID != 1 {
		t.Fatalf("DeletePoll(pollID=%d), want 1", repo.deletedPollID)
	}
}

func TestDeletePoll_RejectsOtherUsersPoll(t *testing.T) {
	// 権限チェックが先に効くので、書き込み可能な部屋でも他人の投票は消せない。
	repo := &fakePollRepoForDelete{poll: &model.Poll{ID: 1, RoomID: 5, AuthorUserID: 7}}
	uc := NewDeletePollUseCase(repo, &fakeRequireWritable{})

	if _, err := uc.Execute(authedCtx(8), 1); err == nil {
		t.Fatal("expected a forbidden error when deleting someone else's poll")
	}
	if repo.deletedPollID != 0 {
		t.Fatal("DeletePoll must not be called for a poll the caller does not own")
	}
}
