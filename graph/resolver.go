package graph

import "github.com/Cityboypenguin/SPACE-server/usecase/user"
import "github.com/Cityboypenguin/SPACE-server/usecase/post"

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	SignUpUseCase   user.SignUpUseCase
	GetUserUseCase  user.GetUserUseCase
	GetUsersUseCase user.GetUsersUseCase

	CreatePostUseCase post.CreatePostUseCase
	UpdatePostUseCase post.UpdatePostUseCase
	GetPostUseCase    post.GetPostUseCase
	GetPostsUseCase   post.GetPostsUseCase
}
