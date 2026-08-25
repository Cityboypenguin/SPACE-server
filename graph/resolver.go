package graph

import (
	"sync/atomic"

	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	analyticsusecase "github.com/Cityboypenguin/SPACE-server/usecase/analytics"
	announcementusecase "github.com/Cityboypenguin/SPACE-server/usecase/announcement"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	"github.com/Cityboypenguin/SPACE-server/usecase/block"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	"github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	favoriteuser "github.com/Cityboypenguin/SPACE-server/usecase/favorite_user"
	inquiryusecase "github.com/Cityboypenguin/SPACE-server/usecase/inquiry"
	mediausecase "github.com/Cityboypenguin/SPACE-server/usecase/media"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	pollusecase "github.com/Cityboypenguin/SPACE-server/usecase/poll"
	"github.com/Cityboypenguin/SPACE-server/usecase/post"
	"github.com/Cityboypenguin/SPACE-server/usecase/profile"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
	reportusecase "github.com/Cityboypenguin/SPACE-server/usecase/report"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	semesterusecase "github.com/Cityboypenguin/SPACE-server/usecase/semester"
	sessionusecase "github.com/Cityboypenguin/SPACE-server/usecase/session"
	systemsettingsusecase "github.com/Cityboypenguin/SPACE-server/usecase/system_settings"
	termsusecase "github.com/Cityboypenguin/SPACE-server/usecase/terms"
	timetableusecase "github.com/Cityboypenguin/SPACE-server/usecase/timetable"
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

	UserUseCases
	PostUseCases

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

	GetFavoriteByIDUseCase                 favorite.GetFavoriteByIDUseCase
	CreateFavoriteUseCase                  favorite.CreateFavoriteUseCase
	DeleteFavoriteUseCase                  favorite.DeleteFavoriteUseCase
	DeleteFavoriteByUserIDAndPostIDUseCase favorite.DeleteFavoriteByUserIDAndPostIDUseCase
	GetFavoriteByUserIDAndPostIDUseCase    favorite.GetFavoriteByUserIDAndPostIDUseCase
	GetFavoritesByPostIDUseCase            favorite.GetFavoritesByPostIDUseCase
	GetFavoritesByUserIDUseCase            favorite.GetFavoritesByUserIDUseCase
	ListFavoritesUseCase                   favorite.ListFavoritesUseCase

	ListMediaByPostIDUseCase mediausecase.ListMediaByPostIDUseCase

	MessageRoomUseCases
	CommunityUseCases
	CourseUseCases
	QuestionUseCases
	PollUseCases

	CreateReportUsecase reportusecase.CreateReportUsecase
	ManageReportUsecase reportusecase.ManageReportUsecase

	NotificationUseCases

	CreateInquiryUsecase inquiryusecase.CreateInquiryUsecase
	ManageInquiryUsecase inquiryusecase.ManageInquiryUsecase

	CreateAnnouncementUseCase *announcementusecase.CreateAnnouncementUseCase
	ListAnnouncementsUseCase  *announcementusecase.ListAnnouncementsUseCase
	GetAnnouncementUseCase    *announcementusecase.GetAnnouncementUseCase
	DeleteAnnouncementUseCase *announcementusecase.DeleteAnnouncementUseCase
	UpdateAnnouncementUseCase *announcementusecase.UpdateAnnouncementUseCase

	SSEBroker *sse.Broker

	PubSub *pubsub.PubSub

	CourseImportTracker *courseimport.Tracker

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
	ListFollowersUseCase           favoriteuser.ListFollowersUseCase
	SearchFavoriteUsersUseCase     favoriteuser.SearchFavoriteUsersUseCase

	CreateTermsUseCase     *termsusecase.CreateTermsUseCase
	GetCurrentTermsUseCase *termsusecase.GetCurrentTermsUseCase
	ConsentToTermsUseCase  *termsusecase.ConsentToTermsUseCase
	CheckConsentUseCase    *termsusecase.CheckConsentUseCase
	ListTermsUseCase       *termsusecase.ListTermsUseCase
	ListConsentsUseCase    *termsusecase.ListConsentsUseCase

	ManageSystemSettingUsecase systemsettingsusecase.ManageSystemSettingUsecase

	GetAnalyticsUseCase          analyticsusecase.GetAnalyticsUseCase
	GetCommunityAnalyticsUseCase analyticsusecase.GetCommunityAnalyticsUseCase
	GetTimeSeriesUseCase         analyticsusecase.GetTimeSeriesUseCase
	RecordSessionUseCase         sessionusecase.RecordSessionUseCase

	// アナリティクスサマリーキャッシュの即時無効化コールバック。
	// 通報ステータス変更など管理操作が DB を更新した直後に呼ぶ。
	InvalidateAnalyticsSummary func()
}

type UserUseCases struct {
	GetUserByIDUseCase            user.GetUserByIDUseCase
	GetUsersByIDsUseCase          user.GetUsersByIDsUseCase
	CreateUserUseCase             user.CreateUserUseCase
	SendEmailOTPUseCase           user.SendEmailOTPUseCase
	VerifyEmailOTPUseCase         user.VerifyEmailOTPUseCase
	ListUsersUseCase              user.ListUsersUseCase
	DeleteUserUseCase             user.DeleteUserUseCase
	UpdateUserUseCase             user.UpdateUserUseCase
	SearchUsersUseCase            user.SearchUsersUseCase
	LoginUserUseCase              user.LoginUserUseCase
	RefreshUserTokenUseCase       user.RefreshUserTokenUseCase
	LogoutUserUseCase             user.LogoutUserUseCase
	FreezeUserUseCase             user.FreezeUserUseCase
	UnfreezeUserUseCase           user.UnfreezeUserUseCase
	RequestPasswordResetUseCase   user.RequestPasswordResetUseCase
	VerifyPasswordResetOTPUseCase user.VerifyPasswordResetOTPUseCase
	ResetPasswordUseCase          user.ResetPasswordUseCase
}

type PostUseCases struct {
	GetPostByIDUseCase                       post.GetPostByIDUseCase
	GetPostsByIDsUseCase                     post.GetPostsByIDsUseCase
	GetRootPostUseCase                       post.GetRootPostUseCase
	GetPostByIDIncludeDeletedUseCase         post.GetPostByIDIncludeDeletedUseCase
	CreatePostUseCase                        post.CreatePostUseCase
	ListPostsUseCase                         post.ListPostsUseCase
	DeletePostUseCase                        post.DeletePostUseCase
	UpdatePostUseCase                        post.UpdatePostUseCase
	SearchPostsUseCase                       post.SearchPostsUseCase
	SearchPostsByHashtagUseCase              post.SearchPostsByHashtagUseCase
	PopularHashtagsUseCase                   post.PopularHashtagsUseCase
	SuggestHashtagsUseCase                   post.SuggestHashtagsUseCase
	ListTopLevelPostsUseCase                 post.ListTopLevelPostsUseCase
	GetFeedPostsUseCase                      post.GetFeedPostsUseCase
	CountNewFeedPostsUseCase                 post.CountNewFeedPostsUseCase
	GetRepliesByIDUseCase                    post.GetRepliesByIDUseCase
	GetRepliesByPostIDsIncludeDeletedUseCase post.GetRepliesByPostIDsIncludeDeletedUseCase
	GetPostsByUserIDUseCase                  post.GetPostsByUserIDUseCase
	GetFavoritePostsByUserIDUseCase          post.GetFavoritePostsByUserIDUseCase
	GetFollowersTopLevelPostsByUserIDUseCase post.GetFollowersTopLevelPostsByUserIDUseCase
}

type MessageRoomUseCases struct {
	GetMessageByIDUseCase           messageusecase.GetMessageByIDUseCase
	SendMessageUseCase              messageusecase.SendMessageUseCase
	ListMessagesUseCase             messageusecase.ListMessagesUseCase
	DeleteMessageUseCase            messageusecase.DeleteMessageUseCase
	UpdateMessageUseCase            messageusecase.UpdateMessageUseCase
	GetLastMessagesByRoomIDsUseCase messageusecase.GetLastMessagesByRoomIDsUseCase
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
	CountUnreadByRoomTypeUseCase    roomusecase.CountUnreadByRoomTypeUseCase
}

type CommunityUseCases struct {
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
}

type CourseUseCases struct {
	SearchCoursesUseCase                 courseusecase.SearchCoursesUseCase
	GetCourseByIDUseCase                 courseusecase.GetCourseByIDUseCase
	RegisterTimetableUseCase             timetableusecase.RegisterTimetableUseCase
	RemoveTimetableUseCase               timetableusecase.RemoveTimetableUseCase
	SetTimetableProfileVisibilityUseCase timetableusecase.SetTimetableProfileVisibilityUseCase
	ListTimetableUseCase                 timetableusecase.ListTimetableUseCase
	GetCurrentSemesterUseCase            semesterusecase.GetCurrentSemesterUseCase
	UpdateCurrentSemesterUseCase         semesterusecase.UpdateCurrentSemesterUseCase
	CheckRoomWritableUseCase             courseusecase.CheckRoomWritableUseCase
	GetOrCreateAnonymousIdentityUseCase  anonusecase.GetOrCreateAnonymousIdentityUseCase
	ImportCoursesUseCase                 courseusecase.ImportCoursesUseCase
	ListCoursesUseCase                   courseusecase.ListCoursesUseCase
	ListCourseYearsUseCase               courseusecase.ListCourseYearsUseCase
}

type QuestionUseCases struct {
	CreateQuestionUseCase   questionusecase.CreateQuestionUseCase
	ListQuestionsUseCase    questionusecase.ListQuestionsUseCase
	GetQuestionByIDUseCase  questionusecase.GetQuestionByIDUseCase
	SelectBestAnswerUseCase questionusecase.SelectBestAnswerUseCase
	AnswerQuestionUseCase   answerusecase.AnswerQuestionUseCase
	ListAnswersUseCase      answerusecase.ListAnswersUseCase
	GetAnswerByIDUseCase    answerusecase.GetAnswerByIDUseCase
}

type PollUseCases struct {
	CreatePollUseCase            pollusecase.CreatePollUseCase
	VotePollUseCase              pollusecase.VotePollUseCase
	ListPollsUseCase             pollusecase.ListPollsUseCase
	GetPollByIDUseCase           pollusecase.GetPollByIDUseCase
	ListPollOptionResultsUseCase pollusecase.ListPollOptionResultsUseCase
}

type NotificationUseCases struct {
	NotificationPublisher                 notificationuc.NotificationPublisher
	ListNotificationsUseCase              notificationuc.ListNotificationsUseCase
	ListNotificationGroupsUseCase         notificationuc.ListNotificationGroupsUseCase
	ListNotificationsByActorUseCase       notificationuc.ListNotificationsByActorUseCase
	GetNotificationUseCase                notificationuc.GetNotificationUseCase
	MarkAsReadUseCase                     notificationuc.MarkAsReadUseCase
	MarkAllAsReadUseCase                  notificationuc.MarkAllAsReadUseCase
	MarkAllAsReadByActorUseCase           notificationuc.MarkAllAsReadByActorUseCase
	CountUnreadUseCase                    notificationuc.CountUnreadUseCase
	DeleteNotificationsUseCase            notificationuc.DeleteNotificationsUseCase
	DeleteReadNotificationsUseCase        notificationuc.DeleteReadNotificationsUseCase
	DeleteReadNotificationsByActorUseCase notificationuc.DeleteReadNotificationsByActorUseCase
}
