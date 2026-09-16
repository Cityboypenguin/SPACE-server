package user

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

const (
	defaultSuggestUsersLimit = 8
	maxSuggestUsersLimit     = 20
)

// SuggestUsersUseCase はメンション (@accountID) 入力中のサジェスト候補を返す。
// クライアントはお気に入りユーザーを先読みしてローカルで解決し、
// そこに無いプレフィックスのときだけこのユースケースを呼ぶ。
type SuggestUsersUseCase interface {
	Execute(ctx context.Context, prefix string, limit int) ([]*model.User, error)
}

var _ SuggestUsersUseCase = &SuggestUsersInteractor{}

type SuggestUsersInteractor struct {
	userRepo repository.UserRepository
}

func NewSuggestUsersUseCase(userRepo repository.UserRepository) SuggestUsersUseCase {
	return &SuggestUsersInteractor{userRepo: userRepo}
}

func (uc *SuggestUsersInteractor) Execute(ctx context.Context, prefix string, limit int) ([]*model.User, error) {
	// 先頭の "@" やスペースが付いていても受け付ける（ハッシュタグのサジェストと同じ扱い）。
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimPrefix(prefix, "@")
	prefix = strings.TrimPrefix(prefix, "＠")
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return []*model.User{}, nil
	}

	if limit <= 0 {
		limit = defaultSuggestUsersLimit
	}
	if limit > maxSuggestUsersLimit {
		limit = maxSuggestUsersLimit
	}

	return uc.userRepo.SuggestUsersByPrefix(ctx, prefix, limit)
}
