package ngword

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/answer"
	"github.com/Cityboypenguin/SPACE-server/usecase/question"
)

// 質問・回答の投稿と編集に、NGワードの検査を前置きする。元のユースケースには手を入れず、
// 組み立て（cmd/server/main.go）で包む。質問箱は授業内チャット専用なので、ルームの種類は見ない。
// 管理者の操作は止めない。

func screen(ctx context.Context, g Guard, roomID *int64, source model.NGWordSource, body string) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}
	if authz.IsAdminRole(claims.Role) {
		return nil
	}
	return g.Screen(ctx, claims.ID, roomID, source, body)
}

type createQuestion struct {
	next question.CreateQuestionUseCase
	g    Guard
}

func GuardCreateQuestion(next question.CreateQuestionUseCase, g Guard) question.CreateQuestionUseCase {
	return &createQuestion{next: next, g: g}
}

func (u *createQuestion) Execute(ctx context.Context, roomID int64, body string, mediaInputs []model.MediaInput) (*model.Question, error) {
	if err := screen(ctx, u.g, &roomID, model.NGWordSourceQuestion, body); err != nil {
		return nil, err
	}
	return u.next.Execute(ctx, roomID, body, mediaInputs)
}

type updateQuestion struct {
	next question.UpdateQuestionUseCase
	g    Guard
}

func GuardUpdateQuestion(next question.UpdateQuestionUseCase, g Guard) question.UpdateQuestionUseCase {
	return &updateQuestion{next: next, g: g}
}

func (u *updateQuestion) Execute(ctx context.Context, questionID int64, body string, deletedMediaIDs []int64) (*model.Question, error) {
	if err := screen(ctx, u.g, nil, model.NGWordSourceQuestion, body); err != nil {
		return nil, err
	}
	return u.next.Execute(ctx, questionID, body, deletedMediaIDs)
}

type answerQuestion struct {
	next answer.AnswerQuestionUseCase
	g    Guard
}

func GuardAnswerQuestion(next answer.AnswerQuestionUseCase, g Guard) answer.AnswerQuestionUseCase {
	return &answerQuestion{next: next, g: g}
}

func (u *answerQuestion) Execute(ctx context.Context, questionID int64, body string, mediaInputs []model.MediaInput) (*model.Answer, error) {
	if err := screen(ctx, u.g, nil, model.NGWordSourceAnswer, body); err != nil {
		return nil, err
	}
	return u.next.Execute(ctx, questionID, body, mediaInputs)
}

type updateAnswer struct {
	next answer.UpdateAnswerUseCase
	g    Guard
}

func GuardUpdateAnswer(next answer.UpdateAnswerUseCase, g Guard) answer.UpdateAnswerUseCase {
	return &updateAnswer{next: next, g: g}
}

func (u *updateAnswer) Execute(ctx context.Context, answerID int64, body string, deletedMediaIDs []int64) (*repository.AnswerWithLikes, error) {
	if err := screen(ctx, u.g, nil, model.NGWordSourceAnswer, body); err != nil {
		return nil, err
	}
	return u.next.Execute(ctx, answerID, body, deletedMediaIDs)
}
