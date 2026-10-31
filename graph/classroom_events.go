package graph

import (
	"context"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	addendumusecase "github.com/Cityboypenguin/SPACE-server/usecase/addendum"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	pollusecase "github.com/Cityboypenguin/SPACE-server/usecase/poll"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
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

// addendumNotifier は質問・回答への追記の通知（NTF-01, NTF-02）。
//
// 授業内チャットは匿名なので、通知に追記した人を載せない（ActorID なし・文言にも出さない）。
// 遷移先は授業の質問箱の、その質問（targetType=question, targetID=質問）。ルームは質問から
// 引けるので、SSE の Extra にだけ roomID / roomType を添える。失敗は送信の成否に影響させず、ログだけ残す。
type addendumNotifier struct {
	pub   notificationuc.NotificationPublisher
	rooms roomusecase.GetRoomUseCase
}

var _ addendumusecase.Notifier = &addendumNotifier{}

func NewAddendumNotifier(pub notificationuc.NotificationPublisher, rooms roomusecase.GetRoomUseCase) *addendumNotifier {
	return &addendumNotifier{pub: pub, rooms: rooms}
}

func (n *addendumNotifier) QuestionAddendumAdded(ctx context.Context, q *model.Question, answererIDs []int64) {
	if len(answererIDs) == 0 {
		return
	}
	room := n.room(ctx, q.RoomID)
	message := notificationuc.MessageQuestionAddendumInRoom(room.Name)
	targetType := notificationuc.TargetQuestion
	questionID := q.ID
	params := make([]notificationuc.PublishParams, 0, len(answererIDs))
	for _, id := range answererIDs {
		params = append(params, notificationuc.PublishParams{
			UserID:     id,
			Type:       notificationuc.TypeQuestionAddendum,
			TargetType: &targetType,
			TargetID:   &questionID,
			Message:    message,
			Extra: map[string]any{
				"roomID":     encodeGraphID("room", q.RoomID),
				"roomType":   room.Type,
				"questionID": encodeGraphID("question", q.ID),
			},
		})
	}
	if err := n.pub.PublishBatch(ctx, params); err != nil {
		logger.Log.Error().Err(err).Str("component", "addendum_notification").
			Int64("question_id", q.ID).Msg("failed to publish question addendum notifications")
	}
}

func (n *addendumNotifier) AnswerAddendumAdded(ctx context.Context, q *model.Question, a *model.Answer) {
	room := n.room(ctx, q.RoomID)
	targetType := notificationuc.TargetQuestion
	questionID := q.ID
	if err := n.pub.Publish(ctx, notificationuc.PublishParams{
		UserID:     q.AskerUserID,
		Type:       notificationuc.TypeAnswerAddendum,
		TargetType: &targetType,
		TargetID:   &questionID,
		Message:    notificationuc.MessageAnswerAddendumInRoom(room.Name),
		Extra: map[string]any{
			"roomID":     encodeGraphID("room", q.RoomID),
			"roomType":   room.Type,
			"questionID": encodeGraphID("question", q.ID),
			"answerID":   encodeGraphID("answer", a.ID),
		},
	}); err != nil {
		logger.Log.Error().Err(err).Str("component", "addendum_notification").
			Int64("answer_id", a.ID).Msg("failed to publish answer addendum notification")
	}
}

// room は通知の文言と遷移先に使うルームを引く。引けなくても通知は出す（授業内チャットなので
// 種別は course、名前は空のまま場所を言わない文言になる）。
func (n *addendumNotifier) room(ctx context.Context, roomID int64) *model.Room {
	room, err := n.rooms.Execute(ctx, roomID)
	if err != nil || room == nil {
		return &model.Room{ID: roomID, Type: model.RoomTypeCourse}
	}
	return room
}
