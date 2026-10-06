package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"

	"golang.org/x/crypto/bcrypt"
)

type LoginUserResult struct {
	AccessToken  string
	RefreshToken string
	// User はログインした本人。自分のメールアドレスは自分に見せてよいので
	// 連絡先を含む UserAccount（GraphQL の UserAuthPayload.user に対応）。
	User *model.UserAccount
	// AccountRestored は、退会手続き中だった本人がログインしたため退会を取り消したか。
	// 画面に「退会を取り消しました」と知らせるために返す。
	AccountRestored bool
}

type LoginUserUseCase interface {
	Execute(ctx context.Context, email, password string) (*LoginUserResult, error)
}

var _ LoginUserUseCase = &LoginUserInteractor{}

type LoginUserInteractor struct {
	userRepo userLoginRepository
}

func NewLoginUserUseCase(userRepo userLoginRepository) LoginUserUseCase {
	return &LoginUserInteractor{userRepo: userRepo}
}

func (uc *LoginUserInteractor) Execute(ctx context.Context, email, password string) (*LoginUserResult, error) {
	// ログインだけが照合用のハッシュを取ってよい経路。
	user, err := uc.userRepo.FindCredentialsByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(password)); err != nil {
		return nil, errors.New("invalid email or password")
	}

	if user.Status == model.UserStatusFrozen {
		return nil, errors.New("account is frozen")
	}

	// 退会手続き中（猶予の間）にログインできたら、退会を取り消す。パスワードの照合は
	// 済んでいるので本人と分かっている。取り消せなかった（ちょうど猶予が切れて
	// 個人情報を消された、など）なら、もう存在しない人として扱う。
	restored := false
	if user.Status == model.UserStatusDeactivated {
		ok, err := uc.userRepo.ReactivateUser(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("invalid email or password")
		}
		restored = true
		user.Status = model.UserStatusActive
	}

	accessToken, err := auth.GenerateUserAccessToken(user.ID, user.Role, user.CredentialsVersion)
	if err != nil {
		return nil, err
	}

	refreshToken, err := auth.GenerateUserRefreshToken(user.ID, user.Role, user.CredentialsVersion)
	if err != nil {
		return nil, err
	}

	// 呼び出し元（リゾルバ）へ返すのはハッシュを外した本人ぶん。
	// ハッシュはこの関数の外へ出さない。
	account := user.UserAccount
	return &LoginUserResult{AccessToken: accessToken, RefreshToken: refreshToken, User: &account, AccountRestored: restored}, nil
}
