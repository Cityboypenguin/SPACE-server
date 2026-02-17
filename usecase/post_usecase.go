package usecase

import (
	"context"
	"github.com/Cityboypenguin/SPACE-server/domain"
	"github.com/Cityboypenguin/SPACE-server/graph/model"
)

type postUsecase struct {
	repo domain.PostRepository
}

func NewPostUsecase(repo domain.PostRepository) domain.PostUsecase {
	return &postUsecase{repo: repo}
}

func (u *postUsecase) CreatePost(ctx context.Context, content string, userId string) (*model.Post, error) {
	post := &model.Post{ID: "P_TEMP", Content: content, UserID: userId}
	return u.repo.Create(ctx, post)
}

func (u *postUsecase) GetPosts(ctx context.Context) ([]*model.Post, error) {
	return u.repo.GetAll(ctx)
}