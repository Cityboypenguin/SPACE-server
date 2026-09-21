package poll

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// EventPublisher は投票の保存に成功したあとの配信の出口（ポート）。
// 位置づけは usecase/question.EventPublisher と同じ。
type EventPublisher interface {
	PollCreated(ctx context.Context, p *model.Poll)
	PollUpdated(ctx context.Context, p *model.Poll)
	PollDeleted(ctx context.Context, p *model.Poll)
}

type noopEventPublisher struct{}

func (noopEventPublisher) PollCreated(context.Context, *model.Poll) {}
func (noopEventPublisher) PollUpdated(context.Context, *model.Poll) {}
func (noopEventPublisher) PollDeleted(context.Context, *model.Poll) {}

func orNoop(p EventPublisher) EventPublisher {
	if p == nil {
		return noopEventPublisher{}
	}
	return p
}
