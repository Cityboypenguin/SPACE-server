package authz

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
)

func IsAdminRole(role string) bool {
	return role == "admin" || role == "administrator"
}

func RequireAuth(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, apperr.Unauthorized("unauthorized")
	}
	return claims, nil
}

func RequireAdmin(ctx context.Context) (*auth.Claims, error) {
	claims, err := RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if !IsAdminRole(claims.Role) {
		return nil, apperr.Forbidden("forbidden")
	}
	return claims, nil
}

func RequireSelfOrAdmin(ctx context.Context, targetUserID int64) (*auth.Claims, error) {
	claims, err := RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if claims.ID != targetUserID && !IsAdminRole(claims.Role) {
		return nil, apperr.Forbidden("forbidden")
	}
	return claims, nil
}
