package graph

import (
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	"github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	"github.com/Cityboypenguin/SPACE-server/usecase/post"
	"github.com/Cityboypenguin/SPACE-server/usecase/profile"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
type Resolver struct {
	GetUserByIDUseCase      user.GetUserByIDUseCase
	CreateUserUseCase       user.CreateUserUseCase
	ListUsersUseCase        user.ListUsersUseCase
	DeleteUserUseCase       user.DeleteUserUseCase
	UpdateUserUseCase       user.UpdateUserUseCase
	SearchUsersUseCase      user.SearchUsersUseCase
	LoginUserUseCase        user.LoginUserUseCase
	RefreshUserTokenUseCase user.RefreshUserTokenUseCase
	LogoutUserUseCase       user.LogoutUserUseCase

	GetAdministratorByIDUseCase      administrator.GetAdministratorByIDUseCase
	CreateAdministratorUseCase       administrator.CreateAdministratorUseCase
	ListAdministratorsUseCase        administrator.ListAdministratorsUseCase
	DeleteAdministratorUseCase       administrator.DeleteAdministratorUseCase
	UpdateAdministratorUseCase       administrator.UpdateAdministratorUseCase
	SearchAdministratorsUseCase      administrator.SearchAdministratorsUseCase
	LoginAdministratorUseCase        administrator.LoginAdministratorUseCase
	LogoutAdministratorUseCase       administrator.LogoutAdministratorUseCase
	RefreshAdministratorTokenUseCase administrator.RefreshAdministratorTokenUseCase

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
	UpdateProfileUseCase                   profile.UpdateProfileUseCase
	GetProfileUseCase                      profile.GetProfileUseCase

	SendMessageUseCase        messageusecase.SendMessageUseCase
	ListMessagesUseCase       messageusecase.ListMessagesUseCase
	CreateRoomUseCase         roomusecase.CreateRoomUseCase
	GetRoomUseCase            roomusecase.GetRoomUseCase
	ListMyDMRoomsUseCase      roomusecase.ListMyDMRoomsUseCase
	GetOrCreateDMRoomUseCase  roomusecase.GetOrCreateDMRoomUseCase
	AddUserToRoomUseCase      roomusecase.AddUserToRoomUseCase
	RemoveUserFromRoomUseCase roomusecase.RemoveUserFromRoomUseCase
	RoomUserRepository        repository.RoomUserRepository
	UserRepository            repository.UserRepository

	PubSub *pubsub.PubSub
}
