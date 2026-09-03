package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type QuestionRepository interface {
	SaveQuestion(ctx context.Context, q *model.Question) error
	GetQuestionByID(ctx context.Context, id int64) (*model.Question, error)
	ListQuestionsByRoomID(ctx context.Context, roomID int64, limit, offset int) ([]*model.Question, int, error)
	// UpdateQuestionBody edits a question's body, scoped to askerUserID so only the
	// asker can edit it. Returns false if no row matched.
	UpdateQuestionBody(ctx context.Context, questionID, askerUserID int64, body string) (bool, error)
	// SetBestAnswer marks a question as answered with the given best answer, scoped to
	// askerUserID so only the asker can select it. It returns false if no row matched
	// (question not found, or the caller is not the asker).
	SetBestAnswer(ctx context.Context, questionID, answerID, askerUserID int64) (bool, error)
	// ClearBestAnswer undoes a previous SetBestAnswer, scoped to askerUserID so only the
	// asker can cancel it. It returns false if no row matched (question not found, or the
	// caller is not the asker).
	ClearBestAnswer(ctx context.Context, questionID, askerUserID int64) (bool, error)
	// DeleteQuestion removes a question (and its answers, via ON DELETE CASCADE). It
	// returns false if no row matched.
	DeleteQuestion(ctx context.Context, questionID int64) (bool, error)
	// DeleteQuestionByAsker removes a question scoped to askerUserID so only the asker
	// can delete it. It returns false if no row matched.
	DeleteQuestionByAsker(ctx context.Context, questionID, askerUserID int64) (bool, error)
}
