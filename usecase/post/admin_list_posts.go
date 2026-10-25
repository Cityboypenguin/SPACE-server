package post

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

// AdminListPostsUseCase は管理画面の投稿一覧（削除済みを含む）。絞り込みは DB 側で行う。
type AdminListPostsUseCase interface {
	Execute(ctx context.Context, f repository.AdminPostFilter, q repository.PageQuery) ([]*model.Post, int, error)
}

var _ AdminListPostsUseCase = &AdminListPostsInteractor{}

type AdminListPostsInteractor struct {
	postRepo repository.PostLister
}

func NewAdminListPostsUseCase(postRepo repository.PostLister) AdminListPostsUseCase {
	return &AdminListPostsInteractor{postRepo: postRepo}
}

func (uc *AdminListPostsInteractor) Execute(ctx context.Context, f repository.AdminPostFilter, q repository.PageQuery) ([]*model.Post, int, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, 0, err
	}
	return uc.postRepo.AdminListPosts(ctx, f, q)
}
