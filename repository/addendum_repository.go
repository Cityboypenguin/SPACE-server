package repository

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// ErrAddendumLimit は、1つの質問・回答に足せる追記の数の上限に達していることを表す。
var ErrAddendumLimit = errors.New("addendum limit reached")

// ErrAddendumTargetNotFound は、追記先の質問・回答が無いことを表す（追記の直前に消された場合）。
var ErrAddendumTargetNotFound = errors.New("addendum target not found")

// AddendumRepository は質問・回答への追記（Q-09, ANS-07）。
type AddendumRepository interface {
	// CreateQuestionAddendum / CreateAnswerAddendum は、その質問・回答の追記が max 件に
	// 達していなければ足す（同じ質問・回答への追記は直列に処理する）。達していれば ErrAddendumLimit、追記先が無ければ ErrAddendumTargetNotFound。
	CreateQuestionAddendum(ctx context.Context, a *model.Addendum, max int) error
	CreateAnswerAddendum(ctx context.Context, a *model.Addendum, max int) error
	// ListQuestionAddenda / ListAnswerAddenda は複数の質問・回答の追記を古い順にまとめて引く
	// （DataLoader 用）。追記が無いものは map に入らない。
	ListQuestionAddenda(ctx context.Context, questionIDs []int64) (map[int64][]*model.Addendum, error)
	ListAnswerAddenda(ctx context.Context, answerIDs []int64) (map[int64][]*model.Addendum, error)
	// DeleteQuestionAddendum / DeleteAnswerAddendum は追記を1件消し、その追記が付いていた
	// 質問・回答のIDを返す。無ければ found が false。
	DeleteQuestionAddendum(ctx context.Context, id int64) (targetID int64, found bool, err error)
	DeleteAnswerAddendum(ctx context.Context, id int64) (targetID int64, found bool, err error)
	// ListAnswererIDs はその質問に回答している人のID（重複なし）。
	ListAnswererIDs(ctx context.Context, questionID int64) ([]int64, error)
}
