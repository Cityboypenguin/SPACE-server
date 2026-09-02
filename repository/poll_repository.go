package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type CreatePollParam struct {
	RoomID              int64
	AuthorUserID        int64
	AuthorRole          string
	Question            string
	AllowMultipleChoice bool
	OptionLabels        []string
}

// PollOptionResult pairs a PollOption with its aggregated vote count and whether the
// viewer has voted for it, as returned by ListOptionsWithResults (computed in one
// query rather than per-option field resolvers, to avoid N+1 queries per poll).
type PollOptionResult struct {
	Option    *model.PollOption
	VoteCount int
	VotedByMe bool
}

type PollRepository interface {
	// CreatePoll creates the Poll and all of its PollOptions in one transaction.
	CreatePoll(ctx context.Context, param CreatePollParam) (*model.Poll, error)
	GetPollByID(ctx context.Context, id int64) (*model.Poll, error)
	ListPollsByRoomID(ctx context.Context, roomID int64, limit, offset int) ([]*model.Poll, int, error)
	ListOptionsWithResults(ctx context.Context, pollID, viewerUserID int64) ([]*PollOptionResult, error)
	// ReplaceVotes atomically clears userID's existing votes on pollID and inserts new
	// votes for optionIDs (only options that actually belong to pollID are accepted,
	// enforced at the SQL level). Used for both single- and multiple-choice polls:
	// re-voting always replaces the previous selection.
	ReplaceVotes(ctx context.Context, pollID, userID int64, optionIDs []int64) error
	// DeletePoll removes a poll (and its options and votes, via ON DELETE CASCADE). It
	// returns false if no row matched.
	DeletePoll(ctx context.Context, pollID int64) (bool, error)
}
