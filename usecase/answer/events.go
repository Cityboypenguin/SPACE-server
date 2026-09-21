package answer

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// EventPublisher は回答の保存に成功したあとの配信の出口（ポート）。
// 位置づけは usecase/question.EventPublisher と同じ。
//
// いいねの付け外しは回答の「更新」として流れる（購読側は同じイベントで作り直すので、
// 専用のイベントを増やしていない）。ベストアンサーの選択・解除は質問側が変わるので
// usecase/question.EventPublisher の QuestionUpdated で流れる。
type EventPublisher interface {
	AnswerCreated(ctx context.Context, a *model.Answer)
	AnswerUpdated(ctx context.Context, a *model.Answer)
	AnswerDeleted(ctx context.Context, a *model.Answer)
}

type noopEventPublisher struct{}

func (noopEventPublisher) AnswerCreated(context.Context, *model.Answer) {}
func (noopEventPublisher) AnswerUpdated(context.Context, *model.Answer) {}
func (noopEventPublisher) AnswerDeleted(context.Context, *model.Answer) {}

func orNoop(p EventPublisher) EventPublisher {
	if p == nil {
		return noopEventPublisher{}
	}
	return p
}
