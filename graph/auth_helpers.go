package graph

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
)

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
