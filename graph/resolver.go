package graph

import (
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	"github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	inquiryusecase "github.com/Cityboypenguin/SPACE-server/usecase/inquiry"
	mediausecase "github.com/Cityboypenguin/SPACE-server/usecase/media"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	"github.com/Cityboypenguin/SPACE-server/usecase/post"
	"github.com/Cityboypenguin/SPACE-server/usecase/profile"
	reportusecase "github.com/Cityboypenguin/SPACE-server/usecase/report"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
type Resolver struct {
	StorageRepository repository.StorageRepository

	GetUserByIDUseCase      user.GetUserByIDUseCase
	CreateUserUseCase       user.CreateUserUseCase
	ListUsersUseCase        user.ListUsersUseCase
	DeleteUserUseCase       user.DeleteUserUseCase
	UpdateUserUseCase       user.UpdateUserUseCase
	SearchUsersUseCase      user.SearchUsersUseCase
	LoginUserUseCase        user.LoginUserUseCase
	RefreshUserTokenUseCase user.RefreshUserTokenUseCase
	LogoutUserUseCase       user.LogoutUserUseCase
	FreezeUserUseCase       user.FreezeUserUseCase
	UnfreezeUserUseCase     user.UnfreezeUserUseCase

	UpdateProfileUseCase profile.UpdateProfileUseCase
	GetProfileUseCase    profile.GetProfileUseCase
	SetAvatarUseCase     profile.SetAvatarUseCase

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

	ListMediaByPostIDUseCase    mediausecase.ListMediaByPostIDUseCase
	ListMediaByMessageIDUseCase mediausecase.ListMediaByMessageIDUseCase

	GetMessageByIDUseCase           messageusecase.GetMessageByIDUseCase
	SendMessageUseCase              messageusecase.SendMessageUseCase
	ListMessagesUseCase             messageusecase.ListMessagesUseCase
	DeleteMessageUseCase            messageusecase.DeleteMessageUseCase
	UpdateMessageUseCase            messageusecase.UpdateMessageUseCase
	CreateRoomUseCase               roomusecase.CreateRoomUseCase
	GetRoomUseCase                  roomusecase.GetRoomUseCase
	GetUserIDsByRoomIDUseCase       roomusecase.GetUserIDsByRoomIDUseCase
	ListUsersByRoomIDsUseCase       roomusecase.ListUsersByRoomIDsUseCase
	ListMyDMRoomsUseCase            roomusecase.ListMyDMRoomsUseCase
	GetOrCreateDMRoomUseCase        roomusecase.GetOrCreateDMRoomUseCase
	AddUserToRoomUseCase            roomusecase.AddUserToRoomUseCase
	RemoveUserFromRoomUseCase       roomusecase.RemoveUserFromRoomUseCase
	DeleteRoomUseCase               roomusecase.DeleteRoomUseCase
	JoinRoomUseCase                 roomusecase.JoinRoomUseCase
	GetRoomUserRoleUseCase          roomusecase.GetRoomUserRoleUseCase
	SetRoomUserRoleUseCase          roomusecase.SetRoomUserRoleUseCase
	ListRoomMembersWithRolesUseCase roomusecase.ListRoomMembersWithRolesUseCase

	CreateCommunityUseCase             communityusecase.CreateCommunityUseCase
	GetCommunityUseCase                communityusecase.GetCommunityUseCase
	UpdateCommunityUseCase             communityusecase.UpdateCommunityUseCase
	SearchCommunityUseCase             communityusecase.SearchCommunityUseCase
	ListMyCommunitiesUseCase           communityusecase.ListMyCommunitiesUseCase
	ListAllCommunitiesUseCase          communityusecase.ListAllCommunitiesUseCase
	PromoteToCommunityOwnerUseCase     communityusecase.PromoteToCommunityOwnerUseCase
	DemoteFromCommunityOwnerUseCase    communityusecase.DemoteFromCommunityOwnerUseCase
	IsSoleOwnerWithOtherMembersUseCase communityusecase.IsSoleOwnerWithOtherMembersUseCase
	GetRandomCommunitiesUseCase        communityusecase.GetRandomCommunitiesUseCase

	CreateReportUsecase reportusecase.CreateReportUsecase
	ManageReportUsecase reportusecase.ManageReportUsecase

	NotificationPublisher    notificationuc.NotificationPublisher
	ListNotificationsUseCase notificationuc.ListNotificationsUseCase
	MarkAsReadUseCase        notificationuc.MarkAsReadUseCase
	MarkAllAsReadUseCase     notificationuc.MarkAllAsReadUseCase
	CountUnreadUseCase       notificationuc.CountUnreadUseCase

	CreateInquiryUsecase inquiryusecase.CreateInquiryUsecase

	PubSub *pubsub.PubSub
}
