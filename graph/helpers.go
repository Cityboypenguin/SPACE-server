package graph

import (
	"context"
	"errors"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/internal/audit"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/opaqueid"
	"github.com/Cityboypenguin/SPACE-server/model"
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
	if !isAdminRole(claims.Role) {
		return nil, errors.New("forbidden")
	}
	return claims, nil
}

func isAdminRole(role string) bool {
	return role == "admin" || role == "administrator"
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
	return nil, errors.New("forbidden")
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

func containsInt64(slice []int64, val int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func (r *queryResolver) buildProfile(ctx context.Context, u *model.User) (*gqlmodel.Profile, error) {
	p, err := r.GetProfileUseCase.Execute(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	if p == nil {
		return &gqlmodel.Profile{
			UserID:    encodeGraphID("user", u.ID),
			User:      toGraphUser(u),
			Username:  u.AccountID,
			CreatedAt: "0",
			UpdatedAt: "0",
		}, nil
	}

	return &gqlmodel.Profile{
		UserID:    encodeGraphID("user", p.UserID),
		User:      toGraphUser(u),
		Username:  u.AccountID,
		Bio:       &p.Bio,
		Image:     &p.Image,
		CreatedAt: p.CreatedAt.Format(timeFormat),
		UpdatedAt: p.UpdatedAt.Format(timeFormat),
	}, nil
}
