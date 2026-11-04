package administrator

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type LogoutAdministratorUseCase interface {
	Execute(ctx context.Context, token string, refreshToken *string) error
}

var _ LogoutAdministratorUseCase = &LogoutAdministratorInteractor{}

type LogoutAdministratorInteractor struct {
	revokedTokenRepo repository.RevokedTokenRepository
}

func NewLogoutAdministratorUseCase(revokedTokenRepo repository.RevokedTokenRepository) LogoutAdministratorUseCase {
	return &LogoutAdministratorInteractor{revokedTokenRepo: revokedTokenRepo}
}

// Execute はアクセストークンを失効させる。refreshToken が渡されれば、それも使えなくする。
//
// 以前はアクセストークンだけを失効させていたので、ログアウトしたあとも
// リフレッシュトークン（30日有効）で新しいアクセストークンをもらえた。
// リフレッシュトークンは更新で1回使うと使用済みになる作りなので、
// 同じ「使用済み」の印を付けて更新に使えなくする。
//
// リフレッシュトークンはアクセストークンより先に片付ける。アクセストークンが
// 期限切れでログアウトが失敗する場合でも、長く使える方は確実に止めたい。
// リフレッシュトークンを持っている人なら誰が止めてもよいので、持ち主の照合はしない。
// 壊れている・期限切れのリフレッシュトークンは、止めるまでもないので無視する。
func (uc *LogoutAdministratorInteractor) Execute(ctx context.Context, token string, refreshToken *string) error {
	if refreshToken != nil && *refreshToken != "" {
		if rc, err := auth.ValidateRefreshToken(*refreshToken); err == nil {
			if _, err := uc.revokedTokenRepo.ConsumeToken(ctx, *refreshToken, rc.ExpiresAt.Unix()); err != nil {
				return err
			}
		}
	}

	claims, err := auth.ValidateToken(token)
	if err != nil {
		return errors.New("invalid token")
	}

	return uc.revokedTokenRepo.RevokeToken(ctx, token, claims.ExpiresAt.Unix())
}
