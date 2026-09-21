package graph

import (
	"context"
	"errors"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
)

// applyRoomReadStatus は既読状態を GraphQL の Room へ写す。
//
// room クエリ・myDMRooms・（将来の一覧）で同じ4項目を並べていたので1箇所にまとめた。
// 特に lastReadMessageID は、どの経路で取った Room かによって入ったり入らなかったり
// すると、クライアントの未読ページ取得の起点が ID と時刻で揺れてしまう。
//
// helpers.go に置くのは、gqlgen generate がリゾルバファイル内の「リゾルバでない
// 関数」をコメントアウトしてしまうため（anonymousUserForCourseRoom と同じ理由）。
func applyRoomReadStatus(gqlRoom *gqlmodel.Room, status *roomusecase.RoomReadStatus) {
	if gqlRoom == nil || status == nil {
		return
	}
	if status.LastReadAt != nil {
		s := time.Unix(*status.LastReadAt, 0).Format(timeFormat)
		gqlRoom.LastReadAt = &s
	}
	if status.LastReadMessageID != nil {
		id := encodeGraphID("message", *status.LastReadMessageID)
		gqlRoom.LastReadMessageID = &id
	}
	gqlRoom.UnreadCount = int32(status.UnreadCount)
	if status.PartnerLastReadAt != nil {
		s := time.Unix(*status.PartnerLastReadAt, 0).Format(timeFormat)
		gqlRoom.PartnerLastReadAt = &s
	}
}

func (r *Resolver) avatarURLFor(p *model.Profile) *string {
	if p == nil || p.AvatarMedia == nil {
		return nil
	}
	url := r.StorageRepository.PublicURL(p.AvatarMedia.StorageKey)
	return &url
}

func (r *Resolver) graphProfile(ctx context.Context, p *model.Profile) (*gqlmodel.Profile, error) {
	if p == nil {
		return nil, errors.New("profile update returned no profile")
	}
	user, err := r.GetUserByIDUseCase.Execute(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("profile user not found after update")
	}
	return toGraphProfile(user, p, r.avatarURLFor(p)), nil
}

func (r *Resolver) communityAvatarURL(c *model.Community) string {
	if c == nil || c.AvatarMedia == nil {
		return ""
	}
	return r.StorageRepository.PublicURL(c.AvatarMedia.StorageKey)
}

// anonymousUserForCourseRoom returns the synthetic User to display for authorUserID
// when roomID is a course-type room (F-05), or nil once we know it is some other
// room type, so the caller falls back to showing the real user. Used by the
// message/question/answer/poll
// user field resolvers. This lives in helpers.go (not schema.resolvers.go) because
// gqlgen comments out any function in the resolver file that isn't a recognized
// resolver stub on every `gqlgen generate` run.
//
// 表示は読み取りだけで完結させる。匿名IDの採番は投稿時（usecase/chat・質問・
// 回答・投票の作成時）に済ませてあるので、ここでは Get しか呼ばない。以前は
// 表示時に GetOrCreate を呼んでいて、読むだけのクエリが DB に書き込む副作用を
// 持っていた。
//
// 実名へ倒すのは「授業ルームではないと分かった」ときだけ。分からなかったとき
// （ルームIDが解けない・ルームが引けない・匿名IDが引けない）は番号なしの「匿名」を
// 返す（理由は anonymousPlaceholderUser のコメント）。
//
// 「分からない」を実名側へ倒してはいけない。実名の漏えいは取り消せないのに対し、
// 非授業ルームが一時的に「匿名」と出るのは、その場かぎりの表示崩れで済む。
// 障害中に DM が「匿名」になるのは目に見えて分かるが、授業ルームが実名になるのは
// 画面上は正常に見えるため誰も気づけない。
//
// ルームも匿名IDも DataLoader 経由で引く。理由は2つある。
//   - 一覧（メッセージ・質問・回答・投票）では項目ごとにこの関数が呼ばれるので、
//     直接引くと項目数ぶんのクエリになる。
//   - Message は userID と user の2フィールドが同じ解決を通るので、1メッセージで
//     2回引いていた。DataLoader のリクエスト内キャッシュで両方とも1回に畳まれ、
//     しかも必ず同じ匿名IDが返る（別々に引くと途中で採番されたときに食い違う）。
//
// 負のキャッシュ（行が無い、を覚えてしまうこと）は問題にならない。この関数に渡る
// のは常に「その部屋に投稿した人」で、投稿時に必ず採番されている（usecase/chat の
// ensureAnonymousIdentity ほか）ため、行が無いのは採番前の古いデータだけ。
func (r *Resolver) anonymousUserForCourseRoom(ctx context.Context, roomID string, authorUserID int64) *gqlmodel.User {
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		// ルームIDが解けないと種別を確かめようがない。自前で符号化したIDなので
		// 通常は起きないが、起きたときに実名を出す口にはしない。
		logChatLookup(err, chatLookupCourseRoom).
			Str("room_graph_id", roomID).
			Int64("author_user_id", authorUserID).
			Msg("failed to decode the room id; hiding the author instead of showing the real user")
		return anonymousPlaceholderUser()
	}

	room, err := dataloader.For(ctx).RoomLoader.Load(ctx, rid)
	if err != nil || room == nil {
		// ここで nil を返すと「授業ルームではない」と同じ扱いになり、匿名のはずの
		// 投稿者が実名で出る。種別が確かめられない以上、授業ルームかもしれないので
		// 匿名側へ倒す。ローダーは不存在を nil で返す（エラーにしない）ので両方ここで拾う。
		logChatLookup(err, chatLookupCourseRoom).
			Int64("room_id", rid).
			Int64("author_user_id", authorUserID).
			Msg("failed to load the room; hiding the author instead of showing the real user")
		return anonymousPlaceholderUser()
	}
	if room.Type != model.RoomTypeCourse {
		return nil
	}

	identity, err := dataloader.For(ctx).AnonymousIdentityLoader.Load(ctx, repository.RoomUserKey{RoomID: rid, UserID: authorUserID})
	if err != nil {
		logChatLookup(err, chatLookupAnonymousLabel).
			Int64("room_id", rid).
			Int64("author_user_id", authorUserID).
			Msg("failed to load anonymous identity for course room")
		return anonymousPlaceholderUser()
	}
	if identity == nil {
		return anonymousPlaceholderUser()
	}
	return toGraphAnonymousUser(identity)
}

func containsInt64(slice []int64, val int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// dmPartnerID は DM のメンバー一覧から「自分ではない相手」を返す。
// 相手が1人に決まらない（退会して自分しか居ない、データ不整合で複数居る）ときは
// 0 と false を返す。
//
// ルームが DM かどうかは room.Type で判断すること。以前はここが「メンバーが2人なら
// DM」という人数からの推測で、2人だけのコミュニティまで DM 扱いになっていた
// （ブロック相手と2人のコミュニティに居ると投稿欄が閉じてしまう）。相手の特定と
// ルーム種別の判定は別の関心事なので、この関数は種別を見ない。
func dmPartnerID(users []*model.User, selfID int64) (int64, bool) {
	var partnerID int64
	found := 0
	for _, u := range users {
		if u.ID == selfID {
			continue
		}
		partnerID = u.ID
		found++
	}
	if found != 1 {
		return 0, false
	}
	return partnerID, true
}

// reportTargetKind は ReportTargetType を opaqueid のカインド文字列に変換する。
func reportTargetKind(t gqlmodel.ReportTargetType) string {
	switch t {
	case gqlmodel.ReportTargetTypePost:
		return "post"
	case gqlmodel.ReportTargetTypeUser:
		return "user"
	case gqlmodel.ReportTargetTypeCommunity:
		return "community"
	default:
		return ""
	}
}

func (r *queryResolver) buildProfile(ctx context.Context, u *model.User) (*gqlmodel.Profile, error) {
	p, err := r.GetProfileUseCase.Execute(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	return toGraphProfile(u, p, r.avatarURLFor(p)), nil
}

func (r *queryResolver) favoriteUsersToGQL(ctx context.Context, favs []*model.FavoriteUser) ([]*gqlmodel.User, error) {
	ids := make([]int64, 0, len(favs))
	for _, f := range favs {
		ids = append(ids, f.FavoriteUserID)
	}
	users, err := r.GetUsersByIDsUseCase.Execute(ctx, ids)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	result := make([]*gqlmodel.User, 0, len(favs))
	for _, f := range favs {
		if u, ok := userMap[f.FavoriteUserID]; ok {
			result = append(result, toGraphUser(u))
		}
	}
	return result, nil
}

func (r *queryResolver) followersToGQL(ctx context.Context, followers []*model.FavoriteUser) ([]*gqlmodel.User, error) {
	ids := make([]int64, 0, len(followers))
	for _, f := range followers {
		ids = append(ids, f.UserID)
	}
	users, err := r.GetUsersByIDsUseCase.Execute(ctx, ids)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	result := make([]*gqlmodel.User, 0, len(followers))
	for _, f := range followers {
		if u, ok := userMap[f.UserID]; ok {
			result = append(result, toGraphUser(u))
		}
	}
	return result, nil
}

func (r *queryResolver) blockedUsersToGQL(ctx context.Context, blockers []*model.Blocker) ([]*gqlmodel.User, error) {
	ids := make([]int64, 0, len(blockers))
	for _, b := range blockers {
		ids = append(ids, b.BlockedUserID)
	}
	users, err := r.GetUsersByIDsUseCase.Execute(ctx, ids)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}
	result := make([]*gqlmodel.User, 0, len(blockers))
	for _, b := range blockers {
		if u, ok := userMap[b.BlockedUserID]; ok {
			result = append(result, toGraphUser(u))
		}
	}
	return result, nil
}

// toNullableInt32 は nil を保ったまま int を GraphQL の Int（*int32）へ変換する。
func toNullableInt32(v *int) *int32 {
	if v == nil {
		return nil
	}
	converted := int32(*v)
	return &converted
}

// toNullableInt は GraphQL の Int（*int32）を nil を保ったまま int へ変換する。
// 妥当でない申告は捨てて nil にし、寸法未取得と同じ扱いに落とす。
func toNullableInt(v *int32) *int {
	if v == nil || !model.ValidImageDimension(int(*v)) {
		return nil
	}
	converted := int(*v)
	return &converted
}

// notificationTargetMessage resolves the Notification/NotificationGroup targetMessage
// field: the chat message a reply notification points at. 削除済み・対象が
// メッセージ以外・いま読む権限が無いなら nil を返す。
//
// 返す Message は messageResolver 経由で解決されるので、授業内チャットなら user
// フィールドは自動的に匿名表示になる。
//
// 通知に対象メッセージのIDが載っているからといって、本文を返してよい理由にはならない。
// 通知は配信時点の権限で作られるので、受け取ってから退出・キックされた利用者の手元にも
// 残る。ここで権限を見ないと、messages クエリなら拒否される本文を、通知の targetMessage
// という別の口から取り出せてしまう。権限判定は messages クエリと同じ
// requireRoomReadAccess（ChatAccess.EnsureReadAccess）を通し、判定を二重に書かない。
//
// 読めないことと消えたことを言い分けない（どちらも nil）。言い分けると「その部屋に
// そのメッセージが在る」ことだけが通知の受け手に伝わってしまう。
func (r *Resolver) notificationTargetMessage(ctx context.Context, targetType *string, targetID *string) (*gqlmodel.Message, error) {
	if targetType == nil || *targetType != notificationTargetTypeMessage || targetID == nil {
		return nil, nil
	}
	numericID, err := decodeGraphID(ctx, "message", *targetID)
	if err != nil {
		return nil, nil
	}
	msg, err := dataloader.For(ctx).MessageLoader.Load(ctx, numericID)
	if err != nil || msg == nil {
		return nil, nil
	}
	if _, err := r.requireRoomReadAccess(ctx, msg.RoomID); err != nil {
		// 退出・キックのあとに古い通知を開いただけ、というのが通常の経路なので
		// Error では残さない（DB障害も同じ枝に入るが、その場合は
		// EnsureReadAccess の内側が Error を出している）。
		logger.Log.Debug().Err(err).
			Str("component", "chat_resolver").
			Str("lookup", chatLookupNotificationTarget).
			Int64("room_id", msg.RoomID).
			Int64("message_id", msg.ID).
			Msg("hiding a notification target message the caller may no longer read")
		return nil, nil
	}
	return toGraphMessage(msg), nil
}

// resolveMentions は本文中のメンション（保存済み）を GraphQL の Mention に変換する。
// メンション先のユーザーは DataLoader 経由でまとめて引く。
// 退会済みなどでユーザーを引けなかったメンションは黙って落とす
// （表示側はそのメンションを通常テキストとして描画する）。
//
// ループの中で Load を1件ずつ呼んではいけない。Load はバッチが閉じるまで
// （WithWait ぶん）ブロックするので、1件目の待ちが明けてから2件目のキーが
// 入る、という直列化が起きて「メンション人数 × 待ち時間」かかる。
// LoadAll は全キーを先に同じバッチへ積んでから待つので、何人居ても待ちは1回。
func resolveMentions(ctx context.Context, mentions []*model.Mention) ([]*gqlmodel.Mention, error) {
	if len(mentions) == 0 {
		return []*gqlmodel.Mention{}, nil
	}

	userIDs := make([]int64, len(mentions))
	for i, m := range mentions {
		userIDs[i] = m.UserID
	}

	users, err := dataloader.For(ctx).UserLoader.LoadAll(ctx, userIDs)
	if err != nil {
		// LoadAll は同じ失敗を人数分束ねて返すので、1件ずつ Load していた頃と
		// 同じ文言になるよう先頭の1件だけ返す。
		return nil, dataloader.FirstError(err)
	}

	result := make([]*gqlmodel.Mention, 0, len(mentions))
	for i, m := range mentions {
		if users[i] == nil {
			continue
		}
		result = append(result, &gqlmodel.Mention{
			User: toGraphUser(users[i]),
			Text: m.Text,
		})
	}
	return result, nil
}
