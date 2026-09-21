package graph

import (
	"context"
	"fmt"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
)

// requireRoomReadAccess verifies the caller may read roomID.
//
// 判定の実体は ChatAccess.EnsureReadAccess にあり、ここはそれを呼ぶだけの薄い
// ラッパ。以前は「授業は全員閲覧可、それ以外は room_users」の判定が messages
// クエリ・room クエリ・この関数・messageSubscription・roomReadStatusUpdated へ
// 写経されていて、管理者の扱いだけ食い違っていた（負債は解消済み）。
// 質問・回答・投票のクエリ／サブスクリプションも、メッセージ系と同じこの1本を通る。
//
// 逆に「閲覧できるか」ではない判定（メンション候補・ルームへの招待や削除・
// コミュニティ権限・既読位置の書き込み）はここへ寄せていない。規則が違うものを
// 同じ関数にまとめると、片方を緩めたときにもう片方まで緩む事故が起きるため。
func (r *Resolver) requireRoomReadAccess(ctx context.Context, roomID int64) (*model.Room, error) {
	return r.ChatAccess.EnsureReadAccess(ctx, roomID)
}

// roomAccessChangedTopic は「このルームの閲覧権限が変わった」合図のトピック。
//
// 他のトピックと同じ `<ルームのID>:<何>:<どうなった>` の形に揃えてある。
// 出す側（publishRoomAccessChanged）と待つ側（roomReadGuard）が同じ文字列を
// 組み立てる必要があるので、1箇所に閉じ込める。
func roomAccessChangedTopic(roomID int64) string {
	return encodeGraphID("room", roomID) + ":access:changed"
}

// roomReadGuard は購読中の再確認に渡す判定と、その合図のトピックを作る。
//
// 判定は開始時と同じ requireRoomReadAccess を呼ぶだけ。同じ1本を通すのが要で、
// 「入るときの条件」と「居続けてよい条件」が別々に書かれると、片方だけ厳しくした
// ときにもう片方が置いていかれる（退出後も購読だけ生き続ける、が正にそれ）。
//
// ctx は購読ごとに渡し直す（転送ループが持っている購読の ctx を使う）。
// ここで閉じ込めてしまうと、購読が切れた後も古い ctx で判定してしまう。
func (r *Resolver) roomReadGuard(roomID int64) subscriptionGuard {
	return subscriptionGuard{
		authorize: func(ctx context.Context) error {
			_, err := r.requireRoomReadAccess(ctx, roomID)
			return err
		},
		revokeTopic: roomAccessChangedTopic(roomID),
	}
}

// publishRoomAccessChanged は「このルームの、この利用者たちの閲覧権限が変わった」
// ことを購読中の全ての台へ知らせる。
//
// 退出・キックのように room_users から行が消える操作の**直後**に、
// トランザクションが確定してから呼ぶこと。確定前に呼ぶと、合図を受けた側が
// 引き直した時点ではまだ行が残っていて、「変わっていない」と判断してしまう。
//
// 配信は取りこぼしてもよい。届かなければ accessRecheckInterval の確かめ直しが
// 拾う（遅れるだけで、漏れ続けはしない）。だからこの関数はエラーを返さない。
func (r *Resolver) publishRoomAccessChanged(roomID int64, userIDs []int64) {
	if len(userIDs) == 0 {
		return
	}
	r.PubSub.Publish(roomAccessChangedTopic(roomID), &RoomAccessChanged{UserIDs: userIDs})
}

// questionSubscription handles the auth/access guard and PubSub fan-out for
// room-scoped question subscriptions (added, updated), mirroring messageSubscription.
func (r *subscriptionResolver) questionSubscription(ctx context.Context, roomID, topic string) (<-chan *gqlmodel.Question, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	if _, err := r.requireRoomReadAccess(ctx, rid); err != nil {
		return nil, err
	}

	return subscribeTopicGuarded(ctx, r.PubSub, topic,
		subscriptionScope{UserID: claims.ID, RoomID: roomID},
		r.roomReadGuard(rid),
		func(q *gqlmodel.Question) (*gqlmodel.Question, bool) { return q, true },
	), nil
}

// roomPollSubscription はルーム単位の投票購読（added / deleted）の認証・権限判定と
// 転送をまとめる。questionSubscription と同じ形（PollUpdated だけは投票IDから
// ルームを引く必要があるのでリゾルバ側に残してある）。
func (r *subscriptionResolver) roomPollSubscription(ctx context.Context, roomID, topic string) (<-chan *gqlmodel.Poll, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	if _, err := r.requireRoomReadAccess(ctx, rid); err != nil {
		return nil, err
	}

	return subscribeTopicGuarded(ctx, r.PubSub, topic,
		subscriptionScope{UserID: claims.ID, RoomID: roomID},
		r.roomReadGuard(rid),
		func(p *gqlmodel.Poll) (*gqlmodel.Poll, bool) { return p, true },
	), nil
}

// answerSubscription handles the auth/access guard and PubSub fan-out for
// question-scoped answer subscriptions (added, updated, deleted), mirroring
// questionSubscription.
func (r *subscriptionResolver) answerSubscription(ctx context.Context, questionID, topic string) (<-chan *gqlmodel.Answer, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	qid, err := decodeGraphID(ctx, "question", questionID)
	if err != nil {
		return nil, fmt.Errorf("invalid question id")
	}
	q, err := r.GetQuestionByIDUseCase.Execute(ctx, qid)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, fmt.Errorf("question not found")
	}
	if _, err := r.requireRoomReadAccess(ctx, q.RoomID); err != nil {
		return nil, err
	}

	return subscribeTopicGuarded(ctx, r.PubSub, topic,
		subscriptionScope{UserID: claims.ID, RoomID: encodeGraphID("room", q.RoomID)},
		r.roomReadGuard(q.RoomID),
		func(a *gqlmodel.Answer) (*gqlmodel.Answer, bool) { return a, true },
	), nil
}

// messageSubscription handles the common auth/membership guard and PubSub fan-out
// for room-scoped message subscriptions (added, deleted, updated).
func (r *subscriptionResolver) messageSubscription(ctx context.Context, roomID, topic string) (<-chan *gqlmodel.Message, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	// 閲覧権限は messages クエリと同じ1本（ChatAccess.EnsureReadAccess）を通す。
	if _, err := r.requireRoomReadAccess(ctx, rid); err != nil {
		return nil, err
	}

	return subscribeTopicGuarded(ctx, r.PubSub, topic,
		subscriptionScope{UserID: claims.ID, RoomID: roomID},
		r.roomReadGuard(rid),
		func(m *gqlmodel.Message) (*gqlmodel.Message, bool) { return m, true },
	), nil
}
