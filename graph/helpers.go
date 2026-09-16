package graph

import (
	"context"
	"errors"
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
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	"github.com/google/uuid"
)

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

// anonymousIdentityForCourseRoom returns the author's per-room anonymous identity
// when roomID is a course-type room (F-05), or nil for every other room type so the
// caller falls back to showing the real user. Used by the message/question/answer
// user field resolvers. This lives in helpers.go (not schema.resolvers.go) because
// gqlgen comments out any function in the resolver file that isn't a recognized
// resolver stub on every `gqlgen generate` run.
func (r *Resolver) anonymousIdentityForCourseRoom(ctx context.Context, roomID string, authorUserID int64) (*model.RoomAnonymousIdentity, error) {
	rid, err := decodeGraphID(ctx, "room", roomID)
	if err != nil {
		return nil, nil
	}

	room, err := r.GetRoomUseCase.Execute(ctx, rid)
	if err != nil || room == nil || room.Type != model.RoomTypeCourse {
		return nil, nil
	}

	return r.GetOrCreateAnonymousIdentityUseCase.Execute(ctx, rid, authorUserID)
}

// requireRoomReadAccess verifies the caller may read roomID: course rooms are open to
// any authenticated user (F-04 "全授業公開"), other room types require room_users
// membership. Used by the question/answer/poll resolvers and subscriptions added in
// Phase 4/5, which are new code paths (unlike the message paths in schema.resolvers.go,
// which keep their own inline checks to avoid changing existing DM/community behavior).
func (r *Resolver) requireRoomReadAccess(ctx context.Context, claims *auth.Claims, roomID int64) (*model.Room, error) {
	room, err := r.GetRoomUseCase.Execute(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room")
	}
	if room != nil && room.Type == model.RoomTypeCourse {
		return room, nil
	}

	memberIDs, err := r.GetUserIDsByRoomIDUseCase.Execute(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify room membership")
	}
	if !containsInt64(memberIDs, claims.ID) {
		return nil, errors.New("forbidden: not a member of this room")
	}
	return room, nil
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
	if _, err := r.requireRoomReadAccess(ctx, claims, rid); err != nil {
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
	if _, err := r.requireRoomReadAccess(ctx, claims, q.RoomID); err != nil {
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

	room, err := r.GetRoomUseCase.Execute(ctx, rid)
	if err != nil {
		return nil, fmt.Errorf("failed to get room")
	}
	if room == nil || room.Type != model.RoomTypeCourse {
		memberIDs, err := r.GetUserIDsByRoomIDUseCase.Execute(ctx, rid)
		if err != nil {
			return nil, fmt.Errorf("failed to verify room membership")
		}
		if !containsInt64(memberIDs, claims.ID) {
			return nil, errors.New("forbidden: not a member of this room")
		}
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

// publishMessageReplyNotification notifies the author of the message that `reply`
// quotes. 自分自身への返信、および返信先が見つからない（削除済み）場合は何もしない。
//
// 授業内チャットは匿名なので、通知文言にはルーム内の匿名ラベルを入れ、actor_id は
// あえて保存しない。actor_id を残すと myNotifications(actorID:) や
// markAllNotificationsAsReadByActor など actor で絞り込むAPIから
// 「匿名NNN = そのユーザー」を突き合わせられてしまい、匿名性が崩れるため。
//
// 遷移先の組み立てにはルームIDと種別が要るが notifications 行は targetType/targetID の
// 1組しか持てないため、SSE には Extra で roomID/roomType を添える（GraphQL 側は
// Notification.targetMessage から辿れる）。
//
// 通知の失敗はメッセージ送信の成否に影響させない（ログのみ）。
//
// 戻り値は通知を送った相手のユーザーID（送らなかったときは nil）。
// 同じ相手をメンションしていたときにメンション通知を二重に送らないために使う。
func (r *Resolver) publishMessageReplyNotification(ctx context.Context, room *model.Room, reply *model.Message, actorID int64) *int64 {
	if reply.ReplyToID == nil {
		return nil
	}

	parent, err := r.GetMessageByIDUseCase.Execute(ctx, *reply.ReplyToID)
	if err != nil || parent == nil {
		return nil
	}
	if parent.UserID == actorID {
		return nil
	}

	message := "あなたのメッセージに返信がありました"
	notificationActorID := &actorID
	if room.Type == model.RoomTypeCourse {
		identity, err := r.GetOrCreateAnonymousIdentityUseCase.Execute(ctx, room.ID, actorID)
		if err != nil {
			logger.Log.Error().Err(err).Msg("failed to resolve anonymous identity for reply notification")
			return nil
		}
		message = fmt.Sprintf("%sさんがあなたのメッセージに返信しました", identity.Label)
		notificationActorID = nil
	}

	targetType := notificationuc.TargetMessage
	if err := r.NotificationPublisher.Publish(ctx, notificationuc.PublishParams{
		UserID:     parent.UserID,
		Type:       notificationuc.TypeMessageReply,
		ActorID:    notificationActorID,
		TargetType: &targetType,
		TargetID:   &reply.ID,
		Message:    message,
		Extra: map[string]any{
			"roomID":   encodeGraphID("room", room.ID),
			"roomType": room.Type,
		},
	}); err != nil {
		logger.Log.Error().Err(err).Msg("failed to publish message reply notification")
		return nil
	}
	return &parent.UserID
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

// publishMentionNotifications はコミュニティチャットでメンションされた各ユーザーへ通知する。
//
// skipUserID には引用返信の通知を既に送った相手を渡す。返信と同時にその相手を
// メンションしても通知が2通にならないようにするため。
// 遷移先の組み立てにはルームIDと種別が要るが notifications 行は targetType/targetID の
// 1組しか持てないため、SSE には Extra で roomID/roomType を添える
// （publishMessageReplyNotification と同じ扱い）。
// 通知の失敗はメッセージ送信の成否に影響させない（ログのみ）。
func (r *Resolver) publishMentionNotifications(ctx context.Context, room *model.Room, msg *model.Message, actorID int64, skipUserID *int64) {
	if len(msg.Mentions) == 0 {
		return
	}

	targetType := notificationuc.TargetMessage
	params := make([]notificationuc.PublishParams, 0, len(msg.Mentions))
	for _, m := range msg.Mentions {
		if skipUserID != nil && m.UserID == *skipUserID {
			continue
		}
		params = append(params, notificationuc.PublishParams{
			UserID:     m.UserID,
			Type:       notificationuc.TypeMessageMention,
			ActorID:    &actorID,
			TargetType: &targetType,
			TargetID:   &msg.ID,
			Message:    "コミュニティであなたがメンションされました",
			Extra: map[string]any{
				"roomID":   encodeGraphID("room", room.ID),
				"roomType": room.Type,
			},
		})
	}
	if len(params) == 0 {
		return
	}

	if err := r.NotificationPublisher.PublishBatch(ctx, params); err != nil {
		logger.Log.Error().Err(err).Msg("failed to publish mention notifications")
	}
}

// resolveMessageMentions はクライアントから届いたメンション先IDを検証済みのメンションに変換する。
//
// ここが担うのは GraphQL ID のデコードだけ。「どのルームでメンションが成立するか」
// 「誰をメンションできるか」の判断は messageusecase.ResolveMentionsUseCase 側にある。
func (r *Resolver) resolveMessageMentions(ctx context.Context, roomID, actorID int64, content string, mentionUserIDs []string) ([]*model.Mention, error) {
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

	return r.ResolveMentionsUseCase.Execute(ctx, roomID, actorID, content, ids)
}
