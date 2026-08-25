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
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/model"
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
