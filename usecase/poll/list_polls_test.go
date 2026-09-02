package poll

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakePollRepoForList struct {
	repository.PollRepository

	polls        []*model.Poll
	total        int
	unvotedTotal int

	gotRoomID       int64
	gotLimit        int
	gotOffset       int
	gotUnvotedRoom  int64
	gotUnvotedUser  int64
	listErr         error
	countUnvotedErr error
}

func (f *fakePollRepoForList) ListPollsByRoomID(_ context.Context, roomID int64, limit, offset int) ([]*model.Poll, int, error) {
	f.gotRoomID = roomID
	f.gotLimit = limit
	f.gotOffset = offset
	return f.polls, f.total, f.listErr
}

func (f *fakePollRepoForList) CountUnvotedPollsByRoomID(_ context.Context, roomID, viewerUserID int64) (int, error) {
	f.gotUnvotedRoom = roomID
	f.gotUnvotedUser = viewerUserID
	return f.unvotedTotal, f.countUnvotedErr
}

func TestListPolls_ReturnsUnvotedTotalForViewer(t *testing.T) {
	repo := &fakePollRepoForList{
		polls:        []*model.Poll{{ID: 1}},
		total:        51,
		unvotedTotal: 12,
	}
	uc := NewListPollsUseCase(repo)

	items, total, unvotedTotal, err := uc.Execute(authedCtx(7), 3, 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || total != 51 || unvotedTotal != 12 {
		t.Fatalf("Execute() = items:%d total:%d unvotedTotal:%d, want items:1 total:51 unvotedTotal:12", len(items), total, unvotedTotal)
	}
	if repo.gotRoomID != 3 || repo.gotLimit != 50 || repo.gotOffset != 0 {
		t.Fatalf("ListPollsByRoomID args = room:%d limit:%d offset:%d, want room:3 limit:50 offset:0", repo.gotRoomID, repo.gotLimit, repo.gotOffset)
	}
	if repo.gotUnvotedRoom != 3 || repo.gotUnvotedUser != 7 {
		t.Fatalf("CountUnvotedPollsByRoomID args = room:%d user:%d, want room:3 user:7", repo.gotUnvotedRoom, repo.gotUnvotedUser)
	}
}
