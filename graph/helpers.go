package graph

import (
	"context"
	"fmt"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// チャット表示まわりで「取れなくても画面は返す」取得の失敗ログに使う lookup 名。
// 配信側の chatDelivery* と同じく、grep する側が経路で絞れるよう1箇所に集める。
const (
	chatLookupCourseRoom      = "course_room_lookup"
	chatLookupAnonymousLabel  = "anonymous_label_lookup"
	chatLookupQuestionRoom    = "question_room_lookup"
	chatLookupPollAuthorRole  = "poll_author_role_lookup"
	chatLookupBlockRelation   = "block_relation_lookup"
	chatLookupBlockedUserIDs  = "blocked_user_ids_lookup"
	chatLookupLastMessages    = "last_messages_lookup"
	chatLookupReadStatus      = "read_status_lookup"
	chatLookupReadStatusBatch = "read_status_batch_lookup"
)

// logChatLookup はリゾルバ側のベストエフォートな取得失敗を、必ず同じ形で残す。
//
// 配信側の logChatDelivery（graph/chat_events.go）と対になる口。どちらも
// 「失敗しても処理は続ける」場所なので、黙って落とすと DB 障害が
// 「未読0」「実名表示」といった正常に見える表示へ化けて誰も気づけない。
// 呼び出し側は room_id / message_id など後から追える情報を足してから Msg すること。
func logChatLookup(err error, lookup string) *zerolog.Event {
	return logger.Log.Error().Err(err).
		Str("component", "chat_resolver").
		Str("lookup", lookup)
}

func (r *Resolver) avatarURLFor(p *model.Profile) *string {
	if p == nil || p.AvatarMedia == nil {
		return nil
	}
	url := r.StorageRepository.PublicURL(p.AvatarMedia.StorageKey)
	return &url
}

func (r *Resolver) communityAvatarURL(c *model.Community) string {
	if c == nil || c.AvatarMedia == nil {
		return ""
	}
	return r.StorageRepository.PublicURL(c.AvatarMedia.StorageKey)
}

// anonymousUserForCourseRoom returns the synthetic User to display for authorUserID
// when roomID is a course-type room (F-05), or nil for every other room type so the
// caller falls back to showing the real user. Used by the message/question/answer/poll
// user field resolvers. This lives in helpers.go (not schema.resolvers.go) because
// gqlgen comments out any function in the resolver file that isn't a recognized
// resolver stub on every `gqlgen generate` run.
//
// 表示は読み取りだけで完結させる。匿名IDの採番は投稿時（usecase/chat・質問・
// 回答・投票の作成時）に済ませてあるので、ここでは Get しか呼ばない。以前は
// 表示時に GetOrCreate を呼んでいて、読むだけのクエリが DB に書き込む副作用を
// 持っていた。
//
// 行が引けなかった場合でも実名にはフォールバックせず、番号なしの「匿名」を返す
// （理由は anonymousPlaceholderUser のコメント）。
func (r *Resolver) anonymousUserForCourseRoom(ctx context.Context, roomID string, authorUserID int64) *gqlmodel.User {
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil
	}

	room, err := r.GetRoomUseCase.Execute(ctx, rid)
	if err != nil {
		// ルーム種別が引けないと「授業ルームではない」と同じ扱いになり、匿名の
		// はずの投稿者が実名で出る。表示を落とすほどではないので nil を返して
		// 従来どおり続けるが、匿名性に関わる失敗なので黙って捨てない。
		logChatLookup(err, chatLookupCourseRoom).
			Int64("room_id", rid).
			Int64("author_user_id", authorUserID).
			Msg("failed to load the room; falling back to showing the real user")
		return nil
	}
	if room == nil || room.Type != model.RoomTypeCourse {
		return nil
	}

	identity, err := r.GetAnonymousIdentityUseCase.Execute(ctx, rid, authorUserID)
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

// questionSubscription handles the auth/access guard and PubSub fan-out for
// room-scoped question subscriptions (added, updated), mirroring messageSubscription.
func (r *subscriptionResolver) questionSubscription(ctx context.Context, roomID, topic string) (<-chan *gqlmodel.Question, error) {
	if _, err := requireAuth(ctx); err != nil {
		return nil, err
	}
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, fmt.Errorf("invalid room id")
	}
	if _, err := r.requireRoomReadAccess(ctx, rid); err != nil {
		return nil, err
	}

	ch := make(chan *gqlmodel.Question, 1)
	sub := r.PubSub.Subscribe(topic)

	go func() {
		defer r.PubSub.Unsubscribe(topic, sub)
		for {
			select {
			case <-ctx.Done():
				close(ch)
				return
			case data, ok := <-sub:
				if !ok {
					close(ch)
					return
				}
				if q, ok := data.(*gqlmodel.Question); ok {
					ch <- q
				}
			}
		}
	}()

	return ch, nil
}

// answerSubscription handles the auth/access guard and PubSub fan-out for
// question-scoped answer subscriptions (added, updated, deleted), mirroring
// questionSubscription.
func (r *subscriptionResolver) answerSubscription(ctx context.Context, questionID, topic string) (<-chan *gqlmodel.Answer, error) {
	if _, err := requireAuth(ctx); err != nil {
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

	ch := make(chan *gqlmodel.Answer, 1)
	sub := r.PubSub.Subscribe(topic)

	go func() {
		defer r.PubSub.Unsubscribe(topic, sub)
		for {
			select {
			case <-ctx.Done():
				close(ch)
				return
			case data, ok := <-sub:
				if !ok {
					close(ch)
					return
				}
				if a, ok := data.(*gqlmodel.Answer); ok {
					ch <- a
				}
			}
		}
	}()

	return ch, nil
}

func requireAuth(ctx context.Context) (*auth.Claims, error) {
	return authz.RequireAuth(ctx)
}

func requireAdminAuth(ctx context.Context) (*auth.Claims, error) {
	return authz.RequireAdmin(ctx)
}

func isAdminRole(role string) bool {
	return authz.IsAdminRole(role)
}

func requireSelfOrAdmin(ctx context.Context, targetUserID int64, action string) (*auth.Claims, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		audit.LogDenied(ctx, action, "user", targetUserID, "unauthorized")
		return nil, err
	}

	if claims.ID == targetUserID || isAdminRole(claims.Role) {
		return claims, nil
	}

	audit.LogDenied(ctx, action, "user", targetUserID, "forbidden")
	return nil, apperr.Forbidden("forbidden")
}

func encodeGraphID(kind string, id int64) string {
	return opaqueid.Encode(kind, id)
}

func decodeGraphID(ctx context.Context, kind string, value string) (int64, error) {
	id, err := opaqueid.Decode(kind, value)
	if err != nil {
		audit.LogProbe(ctx, "decode_id", kind, value, err.Error())
		return 0, err
	}
	return id, nil
}

// isCallerID reports whether userGraphID (an opaque "user"-kind ID, decoded before
// any per-room anonymization is applied) refers to the currently authenticated
// caller. Used to compute isMine on Message/Question/Answer so the frontend can
// tell its own posts apart even when the author is displayed anonymously.
func isCallerID(ctx context.Context, userGraphID string) bool {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return false
	}
	numericID, err := decodeGraphID(ctx, "user", userGraphID)
	if err != nil {
		return false
	}
	return numericID == claims.ID
}

// scheduleTermsBroadcast broadcasts "terms_updated" immediately if effectiveDate is
// in the past or present, or schedules a one-shot timer to fire at effectiveDate.
func scheduleTermsBroadcast(broker *sse.Broker, version string, effectiveDate time.Time) {
	delay := time.Until(effectiveDate)
	if delay <= 0 {
		broker.Broadcast("terms_updated", map[string]any{"version": version})
		return
	}
	time.AfterFunc(delay, func() {
		broker.Broadcast("terms_updated", map[string]any{"version": version})
		logger.Log.Info().Str("version", version).Msg("scheduled terms now effective, SSE broadcast sent")
	})
}

func resolvePagination(limit *int32, offset *int32) (int, int) {
	l := 20
	if limit != nil && *limit > 0 {
		l = int(*limit)
		if l > 100 {
			l = 100
		}
	}
	o := 0
	if offset != nil && *offset > 0 {
		o = int(*offset)
	}
	return l, o
}

func containsInt64(slice []int64, val int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
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

// presignedImageUploadURL generates a presigned put URL for image uploads.
// prefix is the storage path prefix without trailing slash (e.g., "avatars/123").
func (r *queryResolver) presignedImageUploadURL(ctx context.Context, prefix string, maxBytes int64, contentType string) (*gqlmodel.PresignedUploadURL, error) {
	extMap := map[string]string{
		"image/jpeg":    ".jpg",
		"image/png":     ".png",
		"image/webp":    ".webp",
		"image/gif":     ".gif",
		"image/svg+xml": ".svg",
	}
	ext, ok := extMap[contentType]
	if !ok {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
	objectKey := fmt.Sprintf("%s/%s%s", prefix, uuid.New().String(), ext)
	uploadURL, err := r.StorageRepository.PresignedPutURL(ctx, objectKey, contentType, 15*time.Minute, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate upload url")
	}
	return &gqlmodel.PresignedUploadURL{UploadURL: uploadURL, ObjectKey: objectKey}, nil
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

	logger.Log.Info().Str("room_id", roomID).Int64("user_id", claims.ID).Str("topic", topic).Msg("subscription start")

	ch := make(chan *gqlmodel.Message, 1)
	sub := r.PubSub.Subscribe(topic)

	go func() {
		defer r.PubSub.Unsubscribe(topic, sub)
		for {
			select {
			case <-ctx.Done():
				logger.Log.Info().Str("room_id", roomID).Int64("user_id", claims.ID).Str("topic", topic).Str("reason", "context_done").Msg("subscription end")
				close(ch)
				return
			case data, ok := <-sub:
				if !ok {
					logger.Log.Info().Str("room_id", roomID).Int64("user_id", claims.ID).Str("topic", topic).Str("reason", "pubsub_closed").Msg("subscription end")
					close(ch)
					return
				}
				if msg, ok := data.(*gqlmodel.Message); ok {
					ch <- msg
				}
			}
		}
	}()

	return ch, nil
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

// toMediaInputs は GraphQL の添付入力をユースケース層の形へ変換する。
// 添付を受け取る全ミューテーション（投稿・編集・メッセージ・質問・回答）が同じ
// 変換をするため、ここに集約して Media の属性追加時に触る箇所を1つに保つ。
func toMediaInputs(inputs []*gqlmodel.MediaUploadInput) []model.MediaInput {
	var result []model.MediaInput
	for _, m := range inputs {
		if m == nil {
			continue
		}
		result = append(result, model.MediaInput{
			StorageKey:  m.ObjectKey,
			ContentType: m.ContentType,
			Width:       toNullableInt(m.Width),
			Height:      toNullableInt(m.Height),
		})
	}
	return result
}

// notificationTargetMessage resolves the Notification/NotificationGroup targetMessage
// field: the chat message a reply notification points at. 削除済み・対象が
// メッセージ以外なら nil を返す。
//
// 返す Message は messageResolver 経由で解決されるので、授業内チャットなら user
// フィールドは自動的に匿名表示になる。
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
	return toGraphMessage(msg), nil
}

// resolveMentions は本文中のメンション（保存済み）を GraphQL の Mention に変換する。
// メンション先のユーザーは DataLoader 経由でまとめて引く。
// 退会済みなどでユーザーを引けなかったメンションは黙って落とす
// （表示側はそのメンションを通常テキストとして描画する）。
func resolveMentions(ctx context.Context, mentions []*model.Mention) ([]*gqlmodel.Mention, error) {
	result := make([]*gqlmodel.Mention, 0, len(mentions))
	for _, m := range mentions {
		user, err := dataloader.For(ctx).UserLoader.Load(ctx, m.UserID)
		if err != nil {
			return nil, err
		}
		if user == nil {
			continue
		}
		result = append(result, &gqlmodel.Mention{
			User: toGraphUser(user),
			Text: m.Text,
		})
	}
	return result, nil
}

// decodeMentionUserIDs はクライアントから届いたメンション先の GraphQL ID を
// 数値IDへ変換する。
//
// リゾルバが担うのはこのデコードだけ。「どのルームでメンションが成立するか」
// （messageusecase.MentionsSupported: コミュニティのみ）「誰をメンションできるか」の
// 判断は全て messageusecase.ResolveMentionsUseCase 側にあり、それを呼ぶのは
// ChatCommands だけ。リゾルバから直接呼ぶ経路を残さないことで、別経路から
// 授業内チャットに実名メンションが通ることを防いでいる。
func decodeMentionUserIDs(ctx context.Context, mentionUserIDs []string) ([]int64, error) {
	if len(mentionUserIDs) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(mentionUserIDs))
	for _, encoded := range mentionUserIDs {
		id, err := decodeGraphID(ctx, "user", encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid mention user id")
		}
		ids = append(ids, id)
	}
	return ids, nil
}
