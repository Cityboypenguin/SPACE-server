package poll

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
)

type fakePollRepoForVote struct {
	repository.PollRepository
	poll *model.Poll

	gotPollID    int64
	gotUserID    int64
	gotOptionIDs []int64
}

func (f *fakePollRepoForVote) GetPollByID(_ context.Context, _ int64) (*model.Poll, error) {
	return f.poll, nil
}

func (f *fakePollRepoForVote) ReplaceVotes(_ context.Context, pollID, userID int64, optionIDs []int64) error {
	f.gotPollID, f.gotUserID, f.gotOptionIDs = pollID, userID, optionIDs
	return nil
}

// fakeRequireWritable stands in for RequireWritableCourseRoomUseCase (F-06 archive
// enforcement): allow lets a test simulate either a writable course room or a
// rejection (archived / not a course room at all) without depending on the real
// semester-comparison logic, which is covered separately in usecase/course.
type fakeRequireWritable struct {
	err error
}

func (f *fakeRequireWritable) Execute(_ context.Context, _ int64) (*model.Course, error) {
	return nil, f.err
}

var _ course.RequireWritableCourseRoomUseCase = &fakeRequireWritable{}

func authedCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID})
}

func TestVotePoll_RejectsEmptySelection(t *testing.T) {
	uc := NewVotePollUseCase(&fakePollRepoForVote{poll: &model.Poll{ID: 1}}, &fakeRequireWritable{})

	if err := uc.Execute(authedCtx(7), 1, nil); err == nil {
		t.Fatal("expected error when no options are selected")
	}
}

func TestVotePoll_RejectsMultipleOnSingleChoicePoll(t *testing.T) {
	repo := &fakePollRepoForVote{poll: &model.Poll{ID: 1, RoomID: 5, AllowMultipleChoice: false}}
	uc := NewVotePollUseCase(repo, &fakeRequireWritable{})

	err := uc.Execute(authedCtx(7), 1, []int64{10, 11})
	if err == nil {
		t.Fatal("expected error selecting two options on a single-choice poll")
	}
	if repo.gotOptionIDs != nil {
		t.Fatal("ReplaceVotes must not be called when the single-choice constraint is violated")
	}
}

func TestVotePoll_AllowsMultipleOnMultiChoicePoll(t *testing.T) {
	repo := &fakePollRepoForVote{poll: &model.Poll{ID: 1, RoomID: 5, AllowMultipleChoice: true}}
	uc := NewVotePollUseCase(repo, &fakeRequireWritable{})

	if err := uc.Execute(authedCtx(7), 1, []int64{10, 11}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.gotOptionIDs) != 2 {
		t.Fatalf("ReplaceVotes options = %v, want [10 11]", repo.gotOptionIDs)
	}
}

func TestVotePoll_DedupesRepeatedOptionIDs(t *testing.T) {
	// A single-choice poll voted with the same option repeated twice must not be
	// rejected as "multiple options" — the repeat collapses to one real selection.
	repo := &fakePollRepoForVote{poll: &model.Poll{ID: 1, RoomID: 5, AllowMultipleChoice: false}}
	uc := NewVotePollUseCase(repo, &fakeRequireWritable{})

	if err := uc.Execute(authedCtx(7), 1, []int64{10, 10}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.gotOptionIDs) != 1 || repo.gotOptionIDs[0] != 10 {
		t.Fatalf("ReplaceVotes options = %v, want [10]", repo.gotOptionIDs)
	}
}

func TestVotePoll_PropagatesArchiveRejection(t *testing.T) {
	repo := &fakePollRepoForVote{poll: &model.Poll{ID: 1, RoomID: 5}}
	uc := NewVotePollUseCase(repo, &fakeRequireWritable{err: errArchived})

	if err := uc.Execute(authedCtx(7), 1, []int64{10}); err != errArchived {
		t.Fatalf("error = %v, want the archive-check error to be propagated unchanged", err)
	}
	if repo.gotOptionIDs != nil {
		t.Fatal("ReplaceVotes must not be called when the room is not writable")
	}
}

func TestVotePoll_UnknownPollNotFound(t *testing.T) {
	uc := NewVotePollUseCase(&fakePollRepoForVote{poll: nil}, &fakeRequireWritable{})

	if err := uc.Execute(authedCtx(7), 999, []int64{10}); err == nil {
		t.Fatal("expected not-found error for a poll that doesn't exist")
	}
}

var errArchived = &stubError{"archived"}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }
