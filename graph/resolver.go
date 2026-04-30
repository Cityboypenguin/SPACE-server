package graph

import (
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	"github.com/Cityboypenguin/SPACE-server/usecase/comment"
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

	GetAdministratorByIDUseCase administrator.GetAdministratorByIDUseCase
	CreateAdministratorUseCase  administrator.CreateAdministratorUseCase
	ListAdministratorsUseCase   administrator.ListAdministratorsUseCase
	DeleteAdministratorUseCase  administrator.DeleteAdministratorUseCase
	UpdateAdministratorUseCase  administrator.UpdateAdministratorUseCase
	SearchAdministratorsUseCase administrator.SearchAdministratorsUseCase
	LoginAdministratorUseCase   administrator.LoginAdministratorUseCase
	LogoutAdministratorUseCase  administrator.LogoutAdministratorUseCase

	GetPostByIDUseCase      post.GetPostByIDUseCase
	GetPostsByUserIDUseCase post.GetPostsByUserIDUseCase
	CreatePostUseCase       post.CreatePostUseCase
	ListPostsUseCase        post.ListPostsUseCase
	DeletePostUseCase       post.DeletePostUseCase
	UpdatePostUseCase       post.UpdatePostUseCase
	SearchPostsUseCase      post.SearchPostsUseCase

	GetCommentByIDUseCase      comment.GetCommentByIDUseCase
	CreateCommentUseCase       comment.CreateCommentUseCase
	DeleteCommentUseCase       comment.DeleteCommentUseCase
	UpdateCommentUseCase       comment.UpdateCommentUseCase
	GetCommentsByPostIDUseCase comment.GetCommentsByPostIDUseCase
	ListCommentsUseCase        comment.ListCommentsUseCase

	GetFavoriteByIDUseCase           favorite.GetFavoriteByIDUseCase
	CreateFavoriteUseCase            favorite.CreateFavoriteUseCase
	DeleteFavoriteUseCase            favorite.DeleteFavoriteUseCase
	GetFavoritesByPostIDUseCase      favorite.GetFavoritesByPostIDUseCase
	ListFavoritesUseCase             favorite.ListFavoritesUseCase
	GetAdministratorByIDUseCase      administrator.GetAdministratorByIDUseCase
	CreateAdministratorUseCase       administrator.CreateAdministratorUseCase
	ListAdministratorsUseCase        administrator.ListAdministratorsUseCase
	DeleteAdministratorUseCase       administrator.DeleteAdministratorUseCase
	UpdateAdministratorUseCase       administrator.UpdateAdministratorUseCase
	SearchAdministratorsUseCase      administrator.SearchAdministratorsUseCase
	LoginAdministratorUseCase        administrator.LoginAdministratorUseCase
	RefreshAdministratorTokenUseCase administrator.RefreshAdministratorTokenUseCase
	LogoutAdministratorUseCase       administrator.LogoutAdministratorUseCase

	SendMessageUseCase        messageusecase.SendMessageUseCase
	ListMessagesUseCase       messageusecase.ListMessagesUseCase
	CreateRoomUseCase         roomusecase.CreateRoomUseCase
	GetRoomUseCase            roomusecase.GetRoomUseCase
	ListMyDMRoomsUseCase      roomusecase.ListMyDMRoomsUseCase
	GetOrCreateDMRoomUseCase  roomusecase.GetOrCreateDMRoomUseCase
	AddUserToRoomUseCase      roomusecase.AddUserToRoomUseCase
	RemoveUserFromRoomUseCase roomusecase.RemoveUserFromRoomUseCase
	JoinRoomUseCase           roomusecase.JoinRoomUseCase
	RoomUserRepository        repository.RoomUserRepository
	UserRepository            repository.UserRepository

	CreateCommunityUseCase   communityusecase.CreateCommunityUseCase
	GetCommunityUseCase      communityusecase.GetCommunityUseCase
	SearchCommunityUseCase   communityusecase.SearchCommunityUseCase
	ListMyCommunitiesUseCase communityusecase.ListMyCommunitiesUseCase

	PubSub *pubsub.PubSub
}
