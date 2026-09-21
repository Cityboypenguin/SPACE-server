package user

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"

	"golang.org/x/crypto/bcrypt"
)

type LoginUserResult struct {
	AccessToken  string
	RefreshToken string
	// User はログインした本人。自分のメールアドレスは自分に見せてよいので
	// 連絡先を含む UserAccount（GraphQL の UserAuthPayload.user に対応）。
	User *model.UserAccount
}

type LoginUserUseCase interface {
	Execute(ctx context.Context, email, password string) (*LoginUserResult, error)
}

var _ LoginUserUseCase = &LoginUserInteractor{}

type LoginUserInteractor struct {
	userRepo repository.UserCredentialsRepository
}

func NewLoginUserUseCase(userRepo repository.UserCredentialsRepository) LoginUserUseCase {
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
	return &LoginUserResult{AccessToken: accessToken, RefreshToken: refreshToken, User: &account}, nil
}
