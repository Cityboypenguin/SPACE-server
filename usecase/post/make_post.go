package post

import (
	"context"
	"strconv"
	"time"

	gqlmodel "github.com/Cityboypenguin/SPACE-server/graph/model"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type MakePostUseCase interface {
	Execute(context.Context, gqlmodel.CreatePostInput) (*model.Post, error)
}

var _ MakePostUseCase = &SignUpInteractor{}

type SignUpInteractor struct {
	PostRepository repository.PostRepository
}

func (i *SignUpInteractor) Execute(ctx context.Context, in gqlmodel.CreatePostInput) (*model.Post, error) {
	now := time.Now()
	u := &model.Post{}
	id, err := strconv.ParseInt(in.UserID, 10, 64)
	if err != nil {
		return nil, err
	}
	err = u.CreatePost(model.CreatePostParam{
		Content: in.Content,
		Author: model.User{
			ID: id,
		},
		CreatedAt: now,
	})
	if err != nil {
		return nil, err
	}

	if err := i.PostRepository.SavePost(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}
