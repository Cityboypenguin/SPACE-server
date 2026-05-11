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

func LogProbe(ctx context.Context, action string, target string, suppliedID string, reason string) {
	WithActor(logger.Log.Warn().
		Str("event", "audit_probe").
		Str("action", action).
		Str("target", target).
		Str("supplied_id", suppliedID).
		Str("reason", reason), ctx).Send()
}
