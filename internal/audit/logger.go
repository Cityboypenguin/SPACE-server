package audit

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/rs/zerolog"
)

// WithActor enriches a zerolog event with actor identity fields from the request context.
func WithActor(ev *zerolog.Event, ctx context.Context) *zerolog.Event {
	if claims, ok := auth.ClaimsFromContext(ctx); ok {
		return ev.Int64("actor_id", claims.ID).Str("actor_role", claims.Role)
	}
	return ev.Str("actor_id", "anonymous").Str("actor_role", "anonymous")
}

func LogDenied(ctx context.Context, action string, target string, targetID int64, reason string) {
	WithActor(logger.Log.Warn().
		Str("event", "audit_denied").
		Str("action", action).
		Str("target", target).
		Int64("target_id", targetID).
		Str("reason", reason), ctx).Send()
}

// LogMessageDeleted records a successful message deletion for later audit review:
// who deleted it (via WithActor), when (log timestamp), and which message/room.
func LogMessageDeleted(ctx context.Context, roomID int64, messageID int64) {
	WithActor(logger.Log.Info().
		Str("event", "message_deleted").
		Int64("room_id", roomID).
		Int64("message_id", messageID), ctx).Send()
}

// LogCourseChatImageDeleted records an administrator deleting an image from the course chat
// (message / question / answer attachment): who (via WithActor), which media, and where it was attached.
func LogCourseChatImageDeleted(ctx context.Context, mediaID int64, source string, targetID int64, uploaderID int64, removedKey string) {
	WithActor(logger.Log.Info().
		Str("event", "course_chat_image_deleted").
		Int64("media_id", mediaID).
		Str("source", source).
		Int64("target_id", targetID).
		Int64("uploader_id", uploaderID).
		Str("removed_key", removedKey), ctx).Send()
}

// LogCourseChatImageRestored records an administrator putting a removed image back.
func LogCourseChatImageRestored(ctx context.Context, mediaID int64, source string, targetID int64) {
	WithActor(logger.Log.Info().
		Str("event", "course_chat_image_restored").
		Int64("media_id", mediaID).
		Str("source", source).
		Int64("target_id", targetID), ctx).Send()
}

// LogAddendumDeleted records an administrator deleting one addendum of a question or an answer.
func LogAddendumDeleted(ctx context.Context, kind string, addendumID int64, targetID int64) {
	WithActor(logger.Log.Info().
		Str("event", "addendum_deleted").
		Str("kind", kind).
		Int64("addendum_id", addendumID).
		Int64("target_id", targetID), ctx).Send()
}

func LogProbe(ctx context.Context, action string, target string, suppliedID string, reason string) {
	WithActor(logger.Log.Warn().
		Str("event", "audit_probe").
		Str("action", action).
		Str("target", target).
		Str("supplied_id", suppliedID).
		Str("reason", reason), ctx).Send()
}

// LogStampSaved retains the editable values so later changes do not erase the audit record.
func LogStampSaved(ctx context.Context, id string, version int64, before, after map[string]any) {
	WithActor(logger.Log.Info().Str("event", "chat_stamp_saved").Str("stamp_id", id).Int64("version", version).Interface("before", before).Interface("after", after), ctx).Send()
}

func LogStampsReordered(ctx context.Context, reaction bool, ids []string) {
	WithActor(logger.Log.Info().Str("event", "chat_stamps_reordered").Bool("reaction", reaction).Strs("ids", ids), ctx).Send()
}
