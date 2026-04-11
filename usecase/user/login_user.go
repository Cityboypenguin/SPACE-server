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
	Token string
	User  *model.User
}

type LoginUserUseCase interface {
	Execute(ctx context.Context, email, password string) (*LoginUserResult, error)
}

var _ LoginUserUseCase = &LoginUserInteractor{}

type LoginUserInteractor struct {
	userRepo repository.UserRepository
}

func NewLoginUserUseCase(userRepo repository.UserRepository) LoginUserUseCase {
	return &LoginUserInteractor{userRepo: userRepo}
}

func (uc *LoginUserInteractor) Execute(ctx context.Context, email, password string) (*LoginUserResult, error) {
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(password)); err != nil {
		return nil, errors.New("invalid email or password")
	}

	token, err := auth.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, err
	}

	return &LoginUserResult{Token: token, User: user}, nil
}
