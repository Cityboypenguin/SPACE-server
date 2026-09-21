package poll

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// recordingEvents は出た配信を覚える EventPublisher。
type recordingEvents struct {
	created []*model.Poll
	updated []*model.Poll
	deleted []*model.Poll
}

func (r *recordingEvents) PollCreated(_ context.Context, p *model.Poll) {
	r.created = append(r.created, p)
}
func (r *recordingEvents) PollUpdated(_ context.Context, p *model.Poll) {
	r.updated = append(r.updated, p)
}
func (r *recordingEvents) PollDeleted(_ context.Context, p *model.Poll) {
	r.deleted = append(r.deleted, p)
}

// TestDeletePoll_PublishesFromTheUseCase は、配信がユースケース側から出ることを
// 確かめる。
//
// 以前は配信がリゾルバの手順だったので、ユースケースで消しても、リゾルバを
// 通らない経路では購読中の画面に残り続けた。しかもエラーにならないので、
// その経路を足した人は気づけない。
func TestDeletePoll_PublishesFromTheUseCase(t *testing.T) {
	events := &recordingEvents{}
	repo := &fakePollRepoForDelete{poll: &model.Poll{ID: 1, RoomID: 2, AuthorUserID: 42}}
	uc := NewDeletePollUseCase(events, repo, &fakeRequireWritable{})

	if _, err := uc.Execute(authedCtx(42), 1); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(events.deleted) != 1 || events.deleted[0].ID != 1 {
		t.Fatalf("deleted events = %+v, want 1件（ユースケースから配信が出ること）", events.deleted)
	}
}
