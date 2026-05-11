package audit

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
)

func LogDenied(ctx context.Context, action string, target string, targetID int64, reason string) {
	ev := logger.Log.Warn().
		Str("event", "audit_denied").
		Str("action", action).
		Str("target", target).
		Int64("target_id", targetID).
		Str("reason", reason)

	if claims, ok := auth.ClaimsFromContext(ctx); ok {
		ev.Int64("actor_id", claims.ID).Str("actor_role", claims.Role)
	} else {
		ev.Str("actor_id", "anonymous").Str("actor_role", "anonymous")
	}
	ev.Send()
}

func LogProbe(ctx context.Context, action string, target string, suppliedID string, reason string) {
	ev := logger.Log.Warn().
		Str("event", "audit_probe").
		Str("action", action).
		Str("target", target).
		Str("supplied_id", suppliedID).
		Str("reason", reason)

	if claims, ok := auth.ClaimsFromContext(ctx); ok {
		ev.Int64("actor_id", claims.ID).Str("actor_role", claims.Role)
	} else {
		ev.Str("actor_id", "anonymous").Str("actor_role", "anonymous")
	}
	ev.Send()
}
