package auth

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// ValidateAndVerifyToken validates the signature, revocation, and current account state.
// Used by both HTTP middleware and WebSocket init.
//
// # なぜ「凍結しか見ないのに User をまるごと引く」ままなのか
//
// 取っているのは公開情報だけの model.User（パスワードハッシュは UserCredentials 側に
// 分けてあり、この経路には載らない。model/user.go のコメント参照）。ここから更に
// status 列だけを引く専用メソッドを足すことも考えたが、やめた:
//
//   - 主キー1行の取得で、InnoDB はどのみちクラスタ化インデックスから行全体を読む。
//     選ぶ列を減らしても、行を引く回数もインデックスの辿り方も変わらない。
//   - 一方でリポジトリのメソッドとその実装・テスト用の偽物が1つずつ増え、
//     「ユーザーを引く道」が2本になる。認証まわりで道が増えるのは、片方だけ直す
//     事故の芽になる。
//
// 認証1回あたりの往復を減らしたくなったら、列を削るのではなく
// 検証結果そのものを短時間キャッシュする方が効く。ただし凍結・失効の反映が遅れるので、
// そこは別途判断すること。
func ValidateAndVerifyToken(
	ctx context.Context,
	tokenStr string,
	revokedRepo repository.RevokedTokenRepository,
	userRepo repository.UserRepository,
	adminRepo repository.AdministratorRepository,
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

	if claims.Role == "administrator" {
		admin, err := adminRepo.GetAdministratorByID(ctx, claims.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to verify administrator")
		}
		if admin == nil {
			return nil, fmt.Errorf("administrator not found")
		}
	} else {
		u, err := userRepo.GetUserByID(ctx, claims.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to verify user")
		}
		if u == nil {
			return nil, fmt.Errorf("user not found")
		}
		if u.Status == model.UserStatusFrozen {
			return nil, fmt.Errorf("account is frozen")
		}

		version, err := userRepo.GetCredentialsVersionByID(ctx, claims.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to verify token")
		}
		if claims.CredentialsVersion == nil || *claims.CredentialsVersion != version {
			return nil, fmt.Errorf("token has been revoked")
		}
	}

	return claims, nil
}
