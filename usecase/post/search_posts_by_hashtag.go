package post

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type SearchPostsByHashtagUseCase interface {
	Execute(ctx context.Context, tag string) ([]*model.Post, error)
}

var _ SearchPostsByHashtagUseCase = &SearchPostsByHashtagInteractor{}

type SearchPostsByHashtagInteractor struct {
	postRepo repository.PostRepository
}

func NewSearchPostsByHashtagUseCase(postRepo repository.PostRepository) SearchPostsByHashtagUseCase {
	return &SearchPostsByHashtagInteractor{
		postRepo: postRepo,
	}
}

func (uc *SearchPostsByHashtagInteractor) Execute(ctx context.Context, tag string) ([]*model.Post, error) {
	// 先頭の "# " マーカーが付いていても受け付けられるように取り除く。
	tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
	if tag == "" {
		return []*model.Post{}, nil
	}

	posts, err := uc.postRepo.SearchPostsByHashtag(ctx, tag)
	if err != nil {
		return nil, err
	}

	return posts, nil
}
