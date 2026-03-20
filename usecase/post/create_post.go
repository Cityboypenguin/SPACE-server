package post

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreatePostResult struct {
	Post   *model.Post
	Author *model.User
}

type CreatePostUseCase interface {
	Execute(context.Context, model.CreatePostParam) (*CreatePostResult, error)
}

var _ CreatePostUseCase = &CreatePostInteractor{}

type CreatePostInteractor struct {
	PostRepository repository.PostRepository
	UserRepository repository.UserRepository
}

func (i *CreatePostInteractor) Execute(ctx context.Context, param model.CreatePostParam) (*CreatePostResult, error) {
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

	post := model.NewPost(param)

	if err := i.PostRepository.SavePost(ctx, post); err != nil {
		return nil, err
	}

	return &CreatePostResult{Post: post, Author: author}, nil
}
