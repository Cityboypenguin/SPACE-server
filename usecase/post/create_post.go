package post

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreatePostUseCase interface {
	Execute(context.Context, model.CreatePostParam) (*model.Post, error)
}

var _ CreatePostUseCase = &CreatePostInteractor{}

type CreatePostInteractor struct {
	PostRepository repository.PostRepository
	UserRepository repository.UserRepository
}

func (i *CreatePostInteractor) Execute(ctx context.Context, param model.CreatePostParam) (*model.Post, error) {
	if strings.TrimSpace(param.Content) == "" {
		return nil, errors.New("content is required")
	}

	author, err := i.UserRepository.GetUser(ctx, param.AuthorID)
	if err != nil {
		return nil, err
	}
	if author == nil {
		return nil, errors.New("author not found")
	}

	now := time.Now()
	param.CreatedAt = now
	param.UpdatedAt = now

	post := &model.Post{}
	err = post.Create(param)
	if err != nil {
		return nil, err
	}

	err = i.PostRepository.SavePost(ctx, post)
	if err != nil {
		return nil, err
	}

	return post, nil
}
