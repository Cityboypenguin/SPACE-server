package graph

import (
	"context"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	pollusecase "github.com/Cityboypenguin/SPACE-server/usecase/poll"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
)

// classroomEventPublisher は質問・回答・投票の配信アダプタ。
//
// チャット（chatEventPublisher）と同じ位置づけ。ユースケースはドメインの値を
// 渡すだけで、GraphQL 型への変換とトピック名の組み立てはここが持つ
// （不透明IDの作り方も GraphQL 層の都合なので、ユースケースに漏らさない）。
//
// 以前はこの配信がリゾルバの手順だった。保存はユースケース、配信はリゾルバ、と
// 分かれていたので、リゾルバを通らない経路を足すと購読中の画面だけが動かない。
// しかも「動かない」だけでエラーにならないので気づけない。
type classroomEventPublisher struct {
	pubsub chatEventPubSub
}

var (
	_ questionusecase.EventPublisher = &classroomEventPublisher{}
	_ answerusecase.EventPublisher   = &classroomEventPublisher{}
	_ pollusecase.EventPublisher     = &classroomEventPublisher{}
)

// NewClassroomEventPublisher は質問・回答・投票の配信先を繋ぐ。
// 3つのポートを1つの型で満たすのは、配るトピックの組み立て方が同じだから。
func NewClassroomEventPublisher(ps chatEventPubSub) *classroomEventPublisher {
	return &classroomEventPublisher{pubsub: ps}
}

func (p *classroomEventPublisher) QuestionCreated(_ context.Context, q *model.Question) {
	p.publishQuestion(q, "added")
}

func (p *classroomEventPublisher) QuestionUpdated(_ context.Context, q *model.Question) {
	p.publishQuestion(q, "updated")
}

func (p *classroomEventPublisher) QuestionDeleted(_ context.Context, q *model.Question) {
	p.publishQuestion(q, "deleted")
}

func (p *classroomEventPublisher) publishQuestion(q *model.Question, event string) {
	if q == nil {
		return
	}
	p.pubsub.Publish(encodeGraphID("room", q.RoomID)+":question:"+event, toGraphQuestion(q))
}

func (p *classroomEventPublisher) AnswerCreated(_ context.Context, a *model.Answer) {
	p.publishAnswer(a, "added")
}

func (p *classroomEventPublisher) AnswerUpdated(_ context.Context, a *model.Answer) {
	p.publishAnswer(a, "updated")
}

func (p *classroomEventPublisher) AnswerDeleted(_ context.Context, a *model.Answer) {
	p.publishAnswer(a, "deleted")
}

// 回答の購読は質問ごと。トピックが質問IDなのは、1つの質問を開いている画面が
// その質問の回答だけを受け取れるようにするため。
func (p *classroomEventPublisher) publishAnswer(a *model.Answer, event string) {
	if a == nil {
		return
	}
	p.pubsub.Publish(encodeGraphID("question", a.QuestionID)+":answer:"+event, toGraphAnswer(a))
}

func (p *classroomEventPublisher) PollCreated(_ context.Context, poll *model.Poll) {
	p.publishPoll(poll, encodeGraphID("room", pollRoomID(poll))+":poll:added")
}

// 投票の更新だけはトピックが投票ID。1つの投票の結果を見ている画面が、
// 同じルームの別の投票まで受け取らずに済む。
func (p *classroomEventPublisher) PollUpdated(_ context.Context, poll *model.Poll) {
	if poll == nil {
		return
	}
	p.publishPoll(poll, encodeGraphID("poll", poll.ID)+":poll:updated")
}

func (p *classroomEventPublisher) PollDeleted(_ context.Context, poll *model.Poll) {
	p.publishPoll(poll, encodeGraphID("room", pollRoomID(poll))+":poll:deleted")
}

func (p *classroomEventPublisher) publishPoll(poll *model.Poll, topic string) {
	if poll == nil {
		return
	}
	p.pubsub.Publish(topic, toGraphPoll(poll))
}

func pollRoomID(poll *model.Poll) int64 {
	if poll == nil {
		return 0
	}
	return poll.RoomID
}

// 配信内容が GraphQL 型であることの確認用（型が変わったらここで落ちる）。
var (
	_ = (*gqlmodel.Question)(nil)
	_ = (*gqlmodel.Answer)(nil)
	_ = (*gqlmodel.Poll)(nil)
)
