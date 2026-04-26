package graph

import (
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	"github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	"github.com/Cityboypenguin/SPACE-server/usecase/post"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
type Resolver struct {
	GetUserByIDUseCase user.GetUserByIDUseCase
	CreateUserUseCase  user.CreateUserUseCase
	ListUsersUseCase   user.ListUsersUseCase
	DeleteUserUseCase  user.DeleteUserUseCase
	UpdateUserUseCase  user.UpdateUserUseCase
	SearchUsersUseCase user.SearchUsersUseCase
	LoginUserUseCase   user.LoginUserUseCase
	LogoutUserUseCase  user.LogoutUserUseCase

	GetAdministratorByIDUseCase administrator.GetAdministratorByIDUseCase
	CreateAdministratorUseCase  administrator.CreateAdministratorUseCase
	ListAdministratorsUseCase   administrator.ListAdministratorsUseCase
	DeleteAdministratorUseCase  administrator.DeleteAdministratorUseCase
	UpdateAdministratorUseCase  administrator.UpdateAdministratorUseCase
	SearchAdministratorsUseCase administrator.SearchAdministratorsUseCase
	LoginAdministratorUseCase   administrator.LoginAdministratorUseCase
	LogoutAdministratorUseCase  administrator.LogoutAdministratorUseCase

	GetPostByIDUseCase       post.GetPostByIDUseCase
	CreatePostUseCase        post.CreatePostUseCase
	ListPostsUseCase         post.ListPostsUseCase
	DeletePostUseCase        post.DeletePostUseCase
	UpdatePostUseCase        post.UpdatePostUseCase
	SearchPostsUseCase       post.SearchPostsUseCase
	ListTopLevelPostsUseCase post.ListTopLevelPostsUseCase
	GetRepliesByIDUseCase    post.GetRepliesByIDUseCase
	GetPostsByUserIDUseCase  post.GetPostsByUserIDUseCase

	GetFavoriteByIDUseCase                 favorite.GetFavoriteByIDUseCase
	CreateFavoriteUseCase                  favorite.CreateFavoriteUseCase
	DeleteFavoriteUseCase                  favorite.DeleteFavoriteUseCase
	DeleteFavoriteByUserIDAndPostIDUseCase favorite.DeleteFavoriteByUserIDAndPostIDUseCase
	GetFavoriteByUserIDAndPostIDUseCase    favorite.GetFavoriteByUserIDAndPostIDUseCase
	GetFavoritesByPostIDUseCase            favorite.GetFavoritesByPostIDUseCase
	GetFavoritesByUserIDUseCase            favorite.GetFavoritesByUserIDUseCase
	ListFavoritesUseCase                   favorite.ListFavoritesUseCase
}
