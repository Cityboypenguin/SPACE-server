package graph

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
)

func requireAuth(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errors.New("unauthorized")
	}
	return claims, nil
}

func requireAdminAuth(ctx context.Context) (*auth.Claims, error) {
	claims, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if claims.Role != "admin" {
		return nil, errors.New("forbidden")
	}
	return claims, nil
}

func containsInt64(slice []int64, val int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
