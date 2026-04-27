package audit

import (
	"context"
	"log"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
)

func LogDenied(ctx context.Context, action string, target string, targetID int64, reason string) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if ok {
		log.Printf("audit denied action=%s actor_id=%d actor_role=%s target=%s target_id=%d reason=%s", action, claims.ID, claims.Role, target, targetID, reason)
		return
	}

	log.Printf("audit denied action=%s actor_id=anonymous actor_role=anonymous target=%s target_id=%d reason=%s", action, target, targetID, reason)
}

func LogProbe(ctx context.Context, action string, target string, suppliedID string, reason string) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if ok {
		log.Printf("audit probe action=%s actor_id=%d actor_role=%s target=%s supplied_id=%s reason=%s", action, claims.ID, claims.Role, target, suppliedID, reason)
		return
	}

	log.Printf("audit probe action=%s actor_id=anonymous actor_role=anonymous target=%s supplied_id=%s reason=%s", action, target, suppliedID, reason)
}
