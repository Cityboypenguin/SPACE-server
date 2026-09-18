package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// notificationHydration は通知の表示に足す2つの付帯情報（actor と対象 Post）を、
// 「引いたのか、引いていないのか」ごと持ち回すための型。
//
// 以前はリゾルバが GraphQL の選択に関係なく両方を必ず一括取得していた。通知一覧を
// 未読バッジのためだけに引くクライアントにも、users の IN 取得と posts の IN 取得が
// 毎回乗っていた。
//
// 引いていないことを「空の map」で表すと、presenter 側が「引いたが該当なし
// （= 退会済み・削除済み）」と区別できず、要求されてもいないのに退会済みの
// プレースホルダを詰めてしまう。どちらなのかは map の中身ではなく loaded で示す。
type notificationHydration struct {
	// actors は actorID -> ユーザー。actorsLoaded が false なら引いていない。
	actors       map[int64]*model.User
	actorsLoaded bool
	// posts は targetID -> 投稿。postsLoaded が false なら引いていない。
	posts       map[int64]*model.Post
	postsLoaded bool
}

// actor は表示すべき actor を返す。引いていなければ nil（= その通知の actor
// フィールドは要求されていないので、応答には出ない）。
// 引いた上で見つからない場合だけ、従来どおり退会済みのプレースホルダを返す。
func (h notificationHydration) actor(actorID *int64) *model.User {
	if actorID == nil || !h.actorsLoaded {
		return nil
	}
	return h.actors[*actorID]
}

// hydrateNotifications は actor / targetPost のうち要求されたものだけをまとめて引く。
//
// 呼び出し側（通知一覧・通知グループ一覧・単体の通知）が同じ判定を書き写さなくて
// 済むよう1箇所にまとめてある。判定そのものは共通の fieldRequested。
func (r *Resolver) hydrateNotifications(ctx context.Context, actorIDs []int64, postIDs []int64, wantActor, wantPost bool) (notificationHydration, error) {
	h := notificationHydration{}

	if wantActor {
		h.actorsLoaded = true
		h.actors = map[int64]*model.User{}
		if len(actorIDs) > 0 {
			actors, err := r.GetUsersByIDsUseCase.Execute(ctx, actorIDs)
			if err != nil {
				return notificationHydration{}, err
			}
			for _, u := range actors {
				h.actors[u.ID] = u
			}
		}
	}

	if wantPost {
		h.postsLoaded = true
		posts, err := r.buildPostMapByIDs(ctx, postIDs)
		if err != nil {
			return notificationHydration{}, err
		}
		h.posts = posts
	}

	return h, nil
}

// uniqueActorIDs / uniquePostIDs は通知から重複のないIDを拾う。
// 取得するかどうかに関係なく安い（メモリ上のループだけ）ので、呼ぶ側で分岐しない。
func uniqueInt64(ids []*int64) []int64 {
	seen := map[int64]struct{}{}
	var out []int64
	for _, p := range ids {
		if p == nil {
			continue
		}
		if _, ok := seen[*p]; ok {
			continue
		}
		seen[*p] = struct{}{}
		out = append(out, *p)
	}
	return out
}

// postTargetIDs は「対象が Post の通知」のIDを重複なく拾う。
func postTargetIDs(notifications []*model.Notification) []int64 {
	ids := make([]*int64, 0, len(notifications))
	for _, n := range notifications {
		if n.TargetType == nil || *n.TargetType != notificationTargetTypePost {
			continue
		}
		ids = append(ids, n.TargetID)
	}
	return uniqueInt64(ids)
}

// groupPostTargetIDs は postTargetIDs の通知グループ版。
func groupPostTargetIDs(groups []*model.NotificationGroup) []int64 {
	ids := make([]*int64, 0, len(groups))
	for _, g := range groups {
		if g.TargetType == nil || *g.TargetType != notificationTargetTypePost {
			continue
		}
		ids = append(ids, g.TargetID)
	}
	return uniqueInt64(ids)
}

func (r *Resolver) buildPostMapByIDs(ctx context.Context, postIDs []int64) (map[int64]*model.Post, error) {
	postMap := map[int64]*model.Post{}
	if len(postIDs) == 0 {
		return postMap, nil
	}
	posts, err := r.GetPostsByIDsUseCase.Execute(ctx, postIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range posts {
		postMap[p.ID] = p
	}
	return postMap, nil
}
