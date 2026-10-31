// Package addendum は質問・回答への追記（Q-09, ANS-07）。
//
// 一度出した中身は変えられない（質問・回答・メッセージは削除だけ）ので、後から足したい
// ことは追記として別の枠に書く。元の本文は変わらず、追記があったときは関係する人へ
// 通知して、しれっと足されて知らないままにならないようにする（NTF-01, NTF-02, NTF-03）。
package addendum

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	"github.com/Cityboypenguin/SPACE-server/usecase/course"
	"github.com/Cityboypenguin/SPACE-server/usecase/ngword"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
)

// Notifier は追記を知らせる通知の出口（ポート）。ベストエフォートで、失敗は実装側でログに残す。
// 授業内チャットは匿名なので、通知に追記した人を載せない。
type Notifier interface {
	// QuestionAddendumAdded は質問に追記があったとき、その質問に回答している人へ知らせる。
	QuestionAddendumAdded(ctx context.Context, q *model.Question, answererIDs []int64)
	// AnswerAddendumAdded は回答に追記があったとき、質問者へ知らせる。
	AnswerAddendumAdded(ctx context.Context, q *model.Question, a *model.Answer)
}

type Deps struct {
	Questions       repository.QuestionRepository
	Answers         repository.AnswerRepository
	Addenda         repository.AddendumRepository
	RequireWritable course.RequireWritableCourseRoomUseCase
	// NGWords は追記の本文を検査する。nil なら検査しない。
	NGWords ngword.Guard
	// 画面の更新（購読）の配信。質問・回答の「更新」として流す。
	QuestionEvents questionusecase.EventPublisher
	AnswerEvents   answerusecase.EventPublisher
	Notifier       Notifier
}

type Service struct {
	d Deps
}

func NewService(d Deps) *Service {
	if d.Notifier == nil {
		d.Notifier = noopNotifier{}
	}
	return &Service{d: d}
}

type noopNotifier struct{}

func (noopNotifier) QuestionAddendumAdded(context.Context, *model.Question, []int64)     {}
func (noopNotifier) AnswerAddendumAdded(context.Context, *model.Question, *model.Answer) {}

// AddToQuestion は質問に追記する。追記できるのは質問者本人だけ。
func (s *Service) AddToQuestion(ctx context.Context, questionID int64, body string) (*model.Question, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	q, err := s.d.Questions.GetQuestionByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperr.NotFound("質問が見つかりません")
	}
	if q.AskerUserID != claims.ID {
		return nil, apperr.Forbidden("追記できるのは質問した本人だけです")
	}
	if _, err := s.d.RequireWritable.Execute(ctx, q.RoomID); err != nil {
		return nil, err
	}
	a, err := model.NewAddendum(q.ID, claims.ID, body, time.Now())
	if err != nil {
		return nil, err
	}
	roomID := q.RoomID
	if err := ngword.ScreenFor(ctx, s.d.NGWords, claims, &roomID, model.NGWordSourceAddendum, a.Body); err != nil {
		return nil, err
	}
	if err := s.d.Addenda.CreateQuestionAddendum(ctx, a, model.MaxAddendaPerTarget); err != nil {
		return nil, translateLimit(err)
	}

	s.d.QuestionEvents.QuestionUpdated(ctx, q)
	// 通知は回答している人へ（質問者本人は除く）。宛先が引けなくても追記自体は成功している。
	if ids, err := s.d.Addenda.ListAnswererIDs(ctx, q.ID); err == nil {
		others := ids[:0:0]
		for _, id := range ids {
			if id != claims.ID {
				others = append(others, id)
			}
		}
		s.d.Notifier.QuestionAddendumAdded(ctx, q, others)
	}
	return q, nil
}

// AddToAnswer は回答に追記する。追記できるのは回答した本人だけ。
func (s *Service) AddToAnswer(ctx context.Context, answerID int64, body string) (*repository.AnswerWithLikes, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	ans, err := s.d.Answers.GetAnswerByID(ctx, answerID)
	if err != nil {
		return nil, err
	}
	if ans == nil {
		return nil, apperr.NotFound("回答が見つかりません")
	}
	if ans.AuthorUserID != claims.ID {
		return nil, apperr.Forbidden("追記できるのは回答した本人だけです")
	}
	q, err := s.d.Questions.GetQuestionByID(ctx, ans.QuestionID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperr.NotFound("質問が見つかりません")
	}
	if _, err := s.d.RequireWritable.Execute(ctx, q.RoomID); err != nil {
		return nil, err
	}
	a, err := model.NewAddendum(ans.ID, claims.ID, body, time.Now())
	if err != nil {
		return nil, err
	}
	roomID := q.RoomID
	if err := ngword.ScreenFor(ctx, s.d.NGWords, claims, &roomID, model.NGWordSourceAddendum, a.Body); err != nil {
		return nil, err
	}
	if err := s.d.Addenda.CreateAnswerAddendum(ctx, a, model.MaxAddendaPerTarget); err != nil {
		return nil, translateLimit(err)
	}

	s.d.AnswerEvents.AnswerUpdated(ctx, ans)
	// 質問者本人が自分の質問の回答に追記したときは、自分には通知しない。
	if q.AskerUserID != claims.ID {
		s.d.Notifier.AnswerAddendumAdded(ctx, q, ans)
	}
	return s.d.Answers.GetAnswerWithLikesByID(ctx, ans.ID, claims.ID)
}

func translateLimit(err error) error {
	if errors.Is(err, repository.ErrAddendumLimit) {
		return apperr.InvalidInput("追記できる数の上限に達しています")
	}
	return err
}
