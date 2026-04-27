package authz

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
)

func IsAdminRole(role string) bool {
	return role == "admin" || role == "administrator"
}

func RequireAuth(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errors.New("unauthorized")
	}
	return claims, nil
}

func RequireAdmin(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errors.New("unauthorized")
	}
	if !IsAdminRole(claims.Role) {
		return nil, errors.New("forbidden")
	}
	return claims, nil
}

func RequireSelfOrAdmin(ctx context.Context, targetUserID int64) (*auth.Claims, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errors.New("unauthorized")
	}
	if claims.ID != targetUserID && !IsAdminRole(claims.Role) {
		return nil, errors.New("forbidden")
	}
	return claims, nil
}
