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

// denyNotVisible は「権限を判定したい対象の行が引けなかった」を拒否として返す。
//
// 削除済み・ブロック相手などで対象が見えないとき、それが分かる形で返すと
// 「その ID の投稿はあるか」を総当たりで調べられる。requireSelfOrAdmin が
// 権限不足で返すのと同じ forbidden に揃えて、呼び出し側から区別できなくする。
//
// 監査には not_visible として残す。見え方は同じでも、運用で追うときには
// 「権限が無い」と「対象がもう無い」を分けて読めた方がよい。
func denyNotVisible(ctx context.Context, action string, target string, targetID int64) error {
	audit.LogDenied(ctx, action, target, targetID, "not_visible")
	return apperr.Forbidden("forbidden")
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
