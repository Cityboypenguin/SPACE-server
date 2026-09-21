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

// CallerID は認証済みの呼び出し元のIDを返す。
//
// ユースケースが「誰として動くか」を引数で受け取らないための口。引数で受け取る形だと、
// 正しい値を渡す責任が呼び出し側（リゾルバ）に残り続ける。リゾルバが1つなら守れても、
// 別の入口（バッチ・管理用のコマンド・将来の別API）が増えたときに、そこだけ
// 取り違える余地が残る。ctx から取れば、そもそも他人として動かしようがない。
//
// 対象の利用者を指す引数（他人のプロフィールを見る、など）は引き続き引数で渡すこと。
// ここで置き換えるのは「行為者」であって「対象」ではない。
func CallerID(ctx context.Context) (int64, error) {
	claims, err := RequireAuth(ctx)
	if err != nil {
		return 0, err
	}
	return claims.ID, nil
}
