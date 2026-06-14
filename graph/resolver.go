package graph

import (
	"sync/atomic"

	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	announcementusecase "github.com/Cityboypenguin/SPACE-server/usecase/announcement"
	"github.com/Cityboypenguin/SPACE-server/usecase/block"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	"github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	favoriteuser "github.com/Cityboypenguin/SPACE-server/usecase/favorite_user"
	inquiryusecase "github.com/Cityboypenguin/SPACE-server/usecase/inquiry"
	mediausecase "github.com/Cityboypenguin/SPACE-server/usecase/media"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	"github.com/Cityboypenguin/SPACE-server/usecase/post"
	"github.com/Cityboypenguin/SPACE-server/usecase/profile"
	reportusecase "github.com/Cityboypenguin/SPACE-server/usecase/report"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	systemsettingsusecase "github.com/Cityboypenguin/SPACE-server/usecase/system_settings"
	termsusecase "github.com/Cityboypenguin/SPACE-server/usecase/terms"
	"github.com/Cityboypenguin/SPACE-server/usecase/user"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
type Resolver struct {
	StorageRepository     repository.StorageRepository
	MaintenanceRepository repository.MaintenanceRepository
	MaintenanceFlag       *atomic.Bool

	GetUserByIDUseCase      user.GetUserByIDUseCase
	GetUsersByIDsUseCase    user.GetUsersByIDsUseCase
	CreateUserUseCase       user.CreateUserUseCase
	SendEmailOTPUseCase     user.SendEmailOTPUseCase
	ListUsersUseCase        user.ListUsersUseCase
	DeleteUserUseCase       user.DeleteUserUseCase
	UpdateUserUseCase       user.UpdateUserUseCase
	SearchUsersUseCase      user.SearchUsersUseCase
	LoginUserUseCase              user.LoginUserUseCase
	RefreshUserTokenUseCase       user.RefreshUserTokenUseCase
	LogoutUserUseCase             user.LogoutUserUseCase
	FreezeUserUseCase             user.FreezeUserUseCase
	UnfreezeUserUseCase           user.UnfreezeUserUseCase
	RequestPasswordResetUseCase   user.RequestPasswordResetUseCase
	VerifyPasswordResetOTPUseCase user.VerifyPasswordResetOTPUseCase
	ResetPasswordUseCase          user.ResetPasswordUseCase

	UpdateProfileUseCase profile.UpdateProfileUseCase
	GetProfileUseCase    profile.GetProfileUseCase
	SetAvatarUseCase     profile.SetAvatarUseCase
	DeleteAvatarUseCase  profile.DeleteAvatarUseCase

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

	GetPostByIDUseCase                       post.GetPostByIDUseCase
	GetRootPostUseCase                       post.GetRootPostUseCase
	GetPostByIDIncludeDeletedUseCase         post.GetPostByIDIncludeDeletedUseCase
	CreatePostUseCase                        post.CreatePostUseCase
	ListPostsUseCase                         post.ListPostsUseCase
	DeletePostUseCase                        post.DeletePostUseCase
	UpdatePostUseCase                        post.UpdatePostUseCase
	SearchPostsUseCase                       post.SearchPostsUseCase
	ListTopLevelPostsUseCase                 post.ListTopLevelPostsUseCase
	GetFeedPostsUseCase                      post.GetFeedPostsUseCase
	CountNewFeedPostsUseCase                 post.CountNewFeedPostsUseCase
	GetRepliesByIDUseCase                    post.GetRepliesByIDUseCase
	GetRepliesByPostIDsIncludeDeletedUseCase post.GetRepliesByPostIDsIncludeDeletedUseCase
	GetPostsByUserIDUseCase                  post.GetPostsByUserIDUseCase

	GetFavoriteByIDUseCase                 favorite.GetFavoriteByIDUseCase
	CreateFavoriteUseCase                  favorite.CreateFavoriteUseCase
	DeleteFavoriteUseCase                  favorite.DeleteFavoriteUseCase
	DeleteFavoriteByUserIDAndPostIDUseCase favorite.DeleteFavoriteByUserIDAndPostIDUseCase
	GetFavoriteByUserIDAndPostIDUseCase    favorite.GetFavoriteByUserIDAndPostIDUseCase
	GetFavoritesByPostIDUseCase            favorite.GetFavoritesByPostIDUseCase
	GetFavoritesByUserIDUseCase            favorite.GetFavoritesByUserIDUseCase
	ListFavoritesUseCase                   favorite.ListFavoritesUseCase

	ListMediaByPostIDUseCase mediausecase.ListMediaByPostIDUseCase

	GetMessageByIDUseCase              messageusecase.GetMessageByIDUseCase
	SendMessageUseCase                 messageusecase.SendMessageUseCase
	ListMessagesUseCase                messageusecase.ListMessagesUseCase
	DeleteMessageUseCase               messageusecase.DeleteMessageUseCase
	UpdateMessageUseCase               messageusecase.UpdateMessageUseCase
	GetLastMessagesByRoomIDsUseCase    messageusecase.GetLastMessagesByRoomIDsUseCase
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
	MarkRoomAsReadUseCase           roomusecase.MarkRoomAsReadUseCase
	GetRoomReadStatusUseCase        roomusecase.GetRoomReadStatusUseCase
	GetRoomReadStatusBatchUseCase   roomusecase.GetRoomReadStatusBatchUseCase
	GetMembersUnreadCountsUseCase   roomusecase.GetMembersUnreadCountsUseCase

	CreateCommunityUseCase             communityusecase.CreateCommunityUseCase
	GetCommunityUseCase                communityusecase.GetCommunityUseCase
	UpdateCommunityUseCase             communityusecase.UpdateCommunityUseCase
	UpdateCommunityMembersUseCase      communityusecase.UpdateCommunityMembersUseCase
	SearchCommunityUseCase             communityusecase.SearchCommunityUseCase
	ListMyCommunitiesUseCase           communityusecase.ListMyCommunitiesUseCase
	ListAllCommunitiesUseCase          communityusecase.ListAllCommunitiesUseCase
	PromoteToCommunityOwnerUseCase     communityusecase.PromoteToCommunityOwnerUseCase
	DemoteFromCommunityOwnerUseCase    communityusecase.DemoteFromCommunityOwnerUseCase
	IsSoleOwnerWithOtherMembersUseCase communityusecase.IsSoleOwnerWithOtherMembersUseCase
	GetRandomCommunitiesUseCase        communityusecase.GetRandomCommunitiesUseCase

	CreateReportUsecase reportusecase.CreateReportUsecase
	ManageReportUsecase reportusecase.ManageReportUsecase

	NotificationPublisher          notificationuc.NotificationPublisher
	ListNotificationsUseCase       notificationuc.ListNotificationsUseCase
	MarkAsReadUseCase              notificationuc.MarkAsReadUseCase
	MarkAllAsReadUseCase           notificationuc.MarkAllAsReadUseCase
	CountUnreadUseCase             notificationuc.CountUnreadUseCase
	DeleteNotificationsUseCase     notificationuc.DeleteNotificationsUseCase
	DeleteReadNotificationsUseCase notificationuc.DeleteReadNotificationsUseCase

	CreateInquiryUsecase inquiryusecase.CreateInquiryUsecase
	ManageInquiryUsecase inquiryusecase.ManageInquiryUsecase

	CreateAnnouncementUseCase *announcementusecase.CreateAnnouncementUseCase
	ListAnnouncementsUseCase  *announcementusecase.ListAnnouncementsUseCase
	GetAnnouncementUseCase    *announcementusecase.GetAnnouncementUseCase
	DeleteAnnouncementUseCase *announcementusecase.DeleteAnnouncementUseCase
	UpdateAnnouncementUseCase *announcementusecase.UpdateAnnouncementUseCase

	SSEBroker *sse.Broker

	PubSub *pubsub.PubSub

	CreateBlockUseCase            block.BlockUserUseCase
	DeleteBlockUseCase            block.DeleteBlockerUseCase
	GetBlockersByUserIDUseCase    block.GetBlockersByUserIDUseCase
	ListBlockersUseCase           block.ListBlockersUseCase
	SearchBlockersUseCase         block.SearchBlockersUseCase
	CheckBlockRelationUseCase     block.CheckBlockRelationUseCase
	GetBlockRelatedUserIDsUseCase block.GetBlockRelatedUserIDsUseCase

	CreateFavoriteUserUseCase      favoriteuser.CreateFavoriteUserUseCase
	DeleteFavoriteUserUseCase      favoriteuser.DeleteFavoriteUserUseCase
	GetFavoriteUserByUserIDUseCase favoriteuser.GetFavoriteUsersByUserIDUseCase
	ListFavoriteUsersUseCase       favoriteuser.ListFavoriteUsersUseCase
	SearchFavoriteUsersUseCase     favoriteuser.SearchFavoriteUsersUseCase

	CreateTermsUseCase     *termsusecase.CreateTermsUseCase
	GetCurrentTermsUseCase *termsusecase.GetCurrentTermsUseCase
	ConsentToTermsUseCase  *termsusecase.ConsentToTermsUseCase
	CheckConsentUseCase    *termsusecase.CheckConsentUseCase
	ListTermsUseCase       *termsusecase.ListTermsUseCase
	ListConsentsUseCase    *termsusecase.ListConsentsUseCase

	ManageSystemSettingUsecase systemsettingsusecase.ManageSystemSettingUsecase
}
