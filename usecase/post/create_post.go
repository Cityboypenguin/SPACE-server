package post

import (
	"context"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreatePostUseCase interface {
	Execute(context.Context, int64, string) (*model.Post, error)
}

var _ CreatePostUseCase = &CreatePostInteractor{}

type CreatePostInteractor struct {
	PostRepository repository.PostRepository
}

func (i *CreatePostInteractor) Execute(ctx context.Context, authorID int64, content string) (*model.Post, error) {
	post := &model.Post{
		AuthorID:  authorID,
		Content:   content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := i.PostRepository.SavePost(ctx, post)
	if err != nil {
		return nil, err
	}
	return post, nil
}