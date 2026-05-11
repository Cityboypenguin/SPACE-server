package auth

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ValidateAndVerifyToken validates the token signature, checks revocation, and verifies the user
// is not frozen. Used by both HTTP middleware and WebSocket init.
func ValidateAndVerifyToken(
	ctx context.Context,
	tokenStr string,
	revokedRepo repository.RevokedTokenRepository,
	userRepo repository.UserRepository,
) (*Claims, error) {
	claims, err := ValidateAccessToken(tokenStr)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}

	revoked, err := revokedRepo.IsRevoked(ctx, tokenStr)
	if err != nil {
		return nil, fmt.Errorf("failed to verify token")
	}
	if revoked {
		return nil, fmt.Errorf("token has been revoked")
	}

	u, err := userRepo.GetUserByID(ctx, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify user")
	}
	if u != nil && u.Status == model.UserStatusFrozen {
		return nil, fmt.Errorf("account is frozen")
	}

	return claims, nil
}
