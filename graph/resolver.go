package graph

import (
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
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

	UpdateProfileUseCase profile.UpdateProfileUseCase
	GetProfileUseCase    profile.GetProfileUseCase

	GetAdministratorByIDUseCase      administrator.GetAdministratorByIDUseCase
	CreateAdministratorUseCase       administrator.CreateAdministratorUseCase
	CountAdministratorsUseCase       administrator.CountAdministratorsUseCase
	ListAdministratorsUseCase        administrator.ListAdministratorsUseCase
	DeleteAdministratorUseCase       administrator.DeleteAdministratorUseCase
	UpdateAdministratorUseCase       administrator.UpdateAdministratorUseCase
	SearchAdministratorsUseCase      administrator.SearchAdministratorsUseCase
	LoginAdministratorUseCase        administrator.LoginAdministratorUseCase
	RefreshAdministratorTokenUseCase administrator.RefreshAdministratorTokenUseCase
	LogoutAdministratorUseCase       administrator.LogoutAdministratorUseCase

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

	SendMessageUseCase        messageusecase.SendMessageUseCase
	ListMessagesUseCase       messageusecase.ListMessagesUseCase
	DeleteMessageUseCase      messageusecase.DeleteMessageUseCase
	UpdateMessageUseCase      messageusecase.UpdateMessageUseCase
	CreateRoomUseCase         roomusecase.CreateRoomUseCase
	GetRoomUseCase            roomusecase.GetRoomUseCase
	GetUserIDsByRoomIDUseCase roomusecase.GetUserIDsByRoomIDUseCase
	ListUsersByRoomIDsUseCase roomusecase.ListUsersByRoomIDsUseCase
	ListMyDMRoomsUseCase      roomusecase.ListMyDMRoomsUseCase
	GetOrCreateDMRoomUseCase  roomusecase.GetOrCreateDMRoomUseCase
	AddUserToRoomUseCase      roomusecase.AddUserToRoomUseCase
	RemoveUserFromRoomUseCase roomusecase.RemoveUserFromRoomUseCase
	JoinRoomUseCase           roomusecase.JoinRoomUseCase

	CreateCommunityUseCase   communityusecase.CreateCommunityUseCase
	GetCommunityUseCase      communityusecase.GetCommunityUseCase
	SearchCommunityUseCase   communityusecase.SearchCommunityUseCase
	ListMyCommunitiesUseCase communityusecase.ListMyCommunitiesUseCase

	PubSub *pubsub.PubSub
}
