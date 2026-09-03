package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// AnswerWithLikes pairs an Answer with its aggregated like count and whether the
// viewer has liked it, computed in one query per parent question (mirroring
// PollOptionResult) rather than per answer, to avoid N+1 queries.
type AnswerWithLikes struct {
	Answer    *model.Answer
	LikeCount int
	LikedByMe bool
}

type AnswerRepository interface {
	SaveAnswer(ctx context.Context, a *model.Answer) error
	GetAnswerByID(ctx context.Context, id int64) (*model.Answer, error)
	GetAnswerWithLikesByID(ctx context.Context, id, viewerUserID int64) (*AnswerWithLikes, error)
	// ListAnswersWithLikesByQuestionID returns a page of questionID's answers along with
	// their like counts, ordered by like count descending (ties broken by createdAt
	// ascending) so the most-liked answers surface first (F-04-2 いいねの多い回答を上に表示)。
	ListAnswersWithLikesByQuestionID(ctx context.Context, questionID, viewerUserID int64, limit, offset int) ([]*AnswerWithLikes, error)
	// CountAnswersByQuestionID returns questionID's total answer count, for pagination.
	CountAnswersByQuestionID(ctx context.Context, questionID int64) (int, error)
	// UpdateAnswerBody edits an answer's body, scoped to authorUserID so only the
	// author can edit it. Returns false if no row matched (not found, or the caller
	// isn't the author).
	UpdateAnswerBody(ctx context.Context, answerID, authorUserID int64, body string) (bool, error)
	// DeleteAnswer removes an answer, scoped to authorUserID. Returns false if no row
	// matched (not found, or the caller isn't the author).
	DeleteAnswer(ctx context.Context, answerID, authorUserID int64) (bool, error)
	// LikeAnswer is idempotent: liking an already-liked answer is a no-op.
	LikeAnswer(ctx context.Context, answerID, userID int64) error
	// UnlikeAnswer is idempotent: unliking an answer that wasn't liked is a no-op.
	UnlikeAnswer(ctx context.Context, answerID, userID int64) error
}
