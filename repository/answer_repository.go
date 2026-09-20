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

// AnswerPage は1つの質問に対する回答の1ページ分と、その質問の総回答数。
// ListAnswersWithLikesByQuestionID + CountAnswersByQuestionID の戻り値を1つに束ねた形で、
// 質問IDごとのページをまとめて引く ListAnswerPagesByQuestionIDs が使う。
type AnswerPage struct {
	Items []*AnswerWithLikes
	Total int
}

type AnswerRepository interface {
	SaveAnswer(ctx context.Context, a *model.Answer) error
	GetAnswerByID(ctx context.Context, id int64) (*model.Answer, error)
	GetAnswerWithLikesByID(ctx context.Context, id, viewerUserID int64) (*AnswerWithLikes, error)
	// GetAnswersWithLikesByIDs は複数の回答を1クエリでまとめて引く（DataLoader 用）。
	//
	// 返す map には見つかった ID だけを入れる。存在しない ID は key ごと落とす
	// （GetAnswerWithLikesByID が「無ければ nil, nil」を返すのと同じ扱い）。
	GetAnswersWithLikesByIDs(ctx context.Context, ids []int64, viewerUserID int64) (map[int64]*AnswerWithLikes, error)
	// ListAnswerPagesByQuestionIDs は複数の質問について、同じ limit/offset のページと
	// 総件数をまとめて引く（DataLoader 用）。並び順は
	// ListAnswersWithLikesByQuestionID と同一でなければならない。
	//
	// 返す map には回答が1件も無い質問も「Items が空・Total が 0」の
	// *AnswerPage として入れる。ページ自体は「存在しない」ことが無く、
	// nil を返すと呼び出し側が total まで失ってしまうため。
	ListAnswerPagesByQuestionIDs(ctx context.Context, questionIDs []int64, viewerUserID int64, q PageQuery) (map[int64]*AnswerPage, error)
	// ListAnswersWithLikesByQuestionID returns a page of questionID's answers along with
	// their like counts, ordered by like count descending (ties broken by createdAt
	// ascending) so the most-liked answers surface first (F-04-2 いいねの多い回答を上に表示)。
	ListAnswersWithLikesByQuestionID(ctx context.Context, questionID, viewerUserID int64, limit, offset int) ([]*AnswerWithLikes, error)
	// CountAnswersByQuestionIDs は質問ごとの回答件数を1クエリで数える。
	//
	// 件数しか要らない画面（管理画面の質問一覧）のための口。以前はそこが
	// answers(limit: 200) で回答本文と投稿者を全部取り、その len を件数にしていた。
	// 質問200件 × 回答200件で最大4万行が1回の応答に乗っていた。
	//
	// 回答が0件の質問は key ごと欠ける（int のゼロ値がそのまま正しい）。
	CountAnswersByQuestionIDs(ctx context.Context, questionIDs []int64) (map[int64]int, error)
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
