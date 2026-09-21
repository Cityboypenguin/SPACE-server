package question

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// EventPublisher は質問の保存に成功したあとの配信の出口（ポート）。
//
// チャット（usecase/chat.EventPublisher）と同じ考え方で、同じ理由で置いてある。
// 以前は配信がリゾルバの手順だったので、ユースケースで保存しても
// **リゾルバを通らない経路では購読中の画面が動かなかった**。保存した側が
// 配信の責任も持てば、その食い違いが起きない。
//
// GraphQL 型への変換とトピック名の組み立ては実装側（graph のアダプタ）の責務で、
// ここはドメインの値だけ渡す。
//
// ベストエフォート。保存は既にコミット済みなので、配信の失敗で操作を
// 失敗させるわけにいかない。だから error を返さない（失敗は実装側でログに残す）。
type EventPublisher interface {
	QuestionCreated(ctx context.Context, q *model.Question)
	QuestionUpdated(ctx context.Context, q *model.Question)
	QuestionDeleted(ctx context.Context, q *model.Question)
}

// noopEventPublisher は配信先を組み立てない経路（テスト）向け。
type noopEventPublisher struct{}

func (noopEventPublisher) QuestionCreated(context.Context, *model.Question) {}
func (noopEventPublisher) QuestionUpdated(context.Context, *model.Question) {}
func (noopEventPublisher) QuestionDeleted(context.Context, *model.Question) {}

// orNoop は nil を渡されたときに落ちないようにする。
func orNoop(p EventPublisher) EventPublisher {
	if p == nil {
		return noopEventPublisher{}
	}
	return p
}
