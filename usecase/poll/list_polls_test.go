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
	gotWithTotal    bool
	unvotedCalls    int
	gotUnvotedRoom  int64
	gotUnvotedUser  int64
	listErr         error
	countUnvotedErr error
}

func (f *fakePollRepoForList) ListPollsByRoomID(_ context.Context, roomID int64, q repository.PageQuery) ([]*model.Poll, int, error) {
	f.gotRoomID = roomID
	f.gotLimit = q.Limit
	f.gotOffset = q.Offset
	f.gotWithTotal = q.WithTotal
	return f.polls, f.total, f.listErr
}

func (f *fakePollRepoForList) CountUnvotedPollsByRoomID(_ context.Context, roomID, viewerUserID int64) (int, error) {
	f.unvotedCalls++
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

	items, total, unvotedTotal, err := uc.Execute(authedCtx(7), 3, ListPollsQuery{
		Page:             repository.PageQuery{Limit: 50, Offset: 0, WithTotal: true},
		WithUnvotedTotal: true,
	})
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

// unvotedTotal を選んでいないときは NOT EXISTS の COUNT を撃たない。
// 一覧本体より重いクエリなので、「選ばれていないのに毎回走る」に戻ったらここで落ちる。
func TestListPolls_SkipsUnvotedCountWhenNotRequested(t *testing.T) {
	repo := &fakePollRepoForList{
		polls:        []*model.Poll{{ID: 1}},
		total:        51,
		unvotedTotal: 12,
	}
	uc := NewListPollsUseCase(repo)

	_, total, unvotedTotal, err := uc.Execute(authedCtx(7), 3, ListPollsQuery{
		Page:             repository.PageQuery{Limit: 50, WithTotal: true},
		WithUnvotedTotal: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.unvotedCalls != 0 {
		t.Errorf("CountUnvotedPollsByRoomID calls = %d, want 0", repo.unvotedCalls)
	}
	if unvotedTotal != 0 {
		t.Errorf("unvotedTotal = %d, want 0 (not computed)", unvotedTotal)
	}
	if total != 51 {
		t.Errorf("total = %d, want 51 (still counted)", total)
	}
}

// total を要求しないときは PageQuery.WithTotal がそのままリポジトリへ渡る。
func TestListPolls_PassesWithTotalThrough(t *testing.T) {
	repo := &fakePollRepoForList{polls: []*model.Poll{{ID: 1}}}
	uc := NewListPollsUseCase(repo)

	if _, _, _, err := uc.Execute(authedCtx(7), 3, ListPollsQuery{
		Page: repository.PageQuery{Limit: 50, WithTotal: false},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.gotWithTotal {
		t.Error("WithTotal = true, want false")
	}
}
