package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/db"
	"github.com/Cityboypenguin/SPACE-server/graph"
	azurerepo "github.com/Cityboypenguin/SPACE-server/infra/azure"
	infraemail "github.com/Cityboypenguin/SPACE-server/infra/email"
	miniorepo "github.com/Cityboypenguin/SPACE-server/infra/minio"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	infraredis "github.com/Cityboypenguin/SPACE-server/infra/redis"
	infrasmtp "github.com/Cityboypenguin/SPACE-server/infra/smtp"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	authmiddleware "github.com/Cityboypenguin/SPACE-server/internal/middleware"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	announcementusecase "github.com/Cityboypenguin/SPACE-server/usecase/announcement"
	blusecase "github.com/Cityboypenguin/SPACE-server/usecase/block"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	favoriteusecase "github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	fuusecase "github.com/Cityboypenguin/SPACE-server/usecase/favorite_user"
	inquiryusecase "github.com/Cityboypenguin/SPACE-server/usecase/inquiry"
	mediausecase "github.com/Cityboypenguin/SPACE-server/usecase/media"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	postusecase "github.com/Cityboypenguin/SPACE-server/usecase/post"
	profileusecase "github.com/Cityboypenguin/SPACE-server/usecase/profile"
	reportusecase "github.com/Cityboypenguin/SPACE-server/usecase/report"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	systemsettingsusecase "github.com/Cityboypenguin/SPACE-server/usecase/system_settings"
	termsusecase "github.com/Cityboypenguin/SPACE-server/usecase/terms"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	database, err := mysql.New()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to database")
	}

	if err := db.RunMigrations(database); err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to run migrations")
	}
	logger.Log.Info().Msg("database migrations applied")

	e := echo.New()

	isProd := os.Getenv("APP_ENV") == "production"

	// Behind AWS ALB: trust private-network IPs as proxy hops so c.RealIP()
	// returns the real client IP instead of being spoofable via X-Forwarded-For.
	if isProd {
		e.IPExtractor = echo.ExtractIPFromXFFHeader(echo.TrustPrivateNet(true))
	}

	userRepository := mysql.NewMySQLUserRepository(database)
	administratorRepository := mysql.NewMySQLAdministratorRepository(database)
	postRepository := mysql.NewMySQLPostRepository(database)
	favoriteRepository := mysql.NewMySQLFavoriteRepository(database)
	profileRepository := mysql.NewMySQLProfileRepository(database)
	reportRepository := mysql.NewMySQLReportRepository(database)
	favoriteuserRepository := mysql.NewMySQLFavoriteUserRepository(database)
	blockRepository := mysql.NewMySQLBlockRepository(database)
	inquiryRepository := mysql.NewMySQLInquiryRepository(database)
	systemSettingRepository := mysql.NewMySQLSystemSettingRepository(database)
	txManager := mysql.NewMySQLTxManager(database)

	if err := bootstrapInitialAdmin(context.Background(), administratorRepository); err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to bootstrap initial admin")
	}
	if isProd && os.Getenv("INIT_ADMIN_PASSWORD") != "" {
		logger.Log.Warn().Msg("INIT_ADMIN_PASSWORD is set in production; unset it after the initial admin has been created")
	}

	messageRepository, err := mysql.NewMySQLMessageRepository(database)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to initialize message encryption")
	}
	if encryptedCount, err := messageRepository.EncryptPlaintextMessages(context.Background(), 500); err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to encrypt existing message history")
	} else if encryptedCount > 0 {
		logger.Log.Info().Int("count", encryptedCount).Msg("encrypted existing message history")
	}
	mediaRepository := mysql.NewMySQLMediaRepository(database)
	roomRepository := mysql.NewMySQLRoomRepository(database)
	roomUserRepository := mysql.NewMySQLRoomUserRepository(database)
	communityRepository := mysql.NewMySQLCommunityRepository(database)

	var storageRepository repository.StorageRepository
	if os.Getenv("STORAGE_PROVIDER") == "azure" {
		storageRepository, err = azurerepo.New()
		if err != nil {
			logger.Log.Fatal().Err(err).Msg("failed to connect to azure blob storage")
		}
	} else {
		storageRepository, err = miniorepo.New()
		if err != nil {
			logger.Log.Fatal().Err(err).Msg("failed to connect to minio")
		}
	}

	listUsersUseCase := userusecase.NewListUsersUseCase(userRepository)
	deleteUserUseCase := userusecase.NewDeleteUserUseCase(userRepository, postRepository, communityRepository, txManager)
	updateUserUseCase := userusecase.NewUpdateUserUseCase(userRepository)
	getUserByIDUseCase := userusecase.NewGetUserByIDUseCase(userRepository)
	getUsersByIDsUseCase := userusecase.NewGetUsersByIDsUseCase(userRepository)
	searchUsersUseCase := userusecase.NewSearchUsersUseCase(userRepository)
	loginUserUseCase := userusecase.NewLoginUserUseCase(userRepository)
	freezeUserUseCase := userusecase.NewFreezeUserUseCase(userRepository)
	unfreezeUserUseCase := userusecase.NewUnfreezeUserUseCase(userRepository)
	getProfileUseCase := profileusecase.NewGetProfileUseCase(profileRepository)
	updateProfileUseCase := profileusecase.NewUpdateProfileUseCase(profileRepository)
	setAvatarUseCase := profileusecase.NewSetAvatarUseCase(profileRepository, mediaRepository)
	deleteAvatarUseCase := profileusecase.NewDeleteAvatarUseCase(profileRepository)

	createAdministratorUseCase := administrator.NewCreateAdministratorUseCase(administratorRepository)
	countAdministratorsUseCase := administrator.NewCountAdministratorsUseCase(administratorRepository)
	getAdministratorByIDUseCase := administrator.NewGetAdministratorByIDUseCase(administratorRepository)
	listAdministratorsUseCase := administrator.NewListAdministratorsUseCase(administratorRepository)
	deleteAdministratorUseCase := administrator.NewDeleteAdministratorUseCase(administratorRepository)
	updateAdministratorUseCase := administrator.NewUpdateAdministratorUseCase(administratorRepository)
	searchAdministratorsUseCase := administrator.NewSearchAdministratorsUseCase(administratorRepository)
	loginAdministratorUseCase := administrator.NewLoginAdministratorUseCase(administratorRepository)

	createPostUseCase := postusecase.NewCreatePostUseCase(postRepository, mediaRepository, txManager)
	updatePostUseCase := postusecase.NewUpdatePostUseCase(postRepository, mediaRepository, txManager)
	deletePostUseCase := postusecase.NewDeletePostUseCase(postRepository)
	getPostByIDUseCase := postusecase.NewGetPostByIDUseCase(postRepository)
	getRootPostUseCase := postusecase.NewGetRootPostUseCase(postRepository)
	getPostByIDIncludeDeletedUseCase := postusecase.NewGetPostByIDIncludeDeletedUseCase(postRepository)
	listPostsUseCase := postusecase.NewListPostsUseCase(postRepository)
	searchPostsUseCase := postusecase.NewSearchPostsUseCase(postRepository)
	getPostsByUserIDUseCase := postusecase.NewGetPostsByUserIDUseCase(postRepository)
	getRepliesByIDUseCase := postusecase.NewGetRepliesByIDUseCase(postRepository)
	getRepliesByPostIDsIncludeDeletedUseCase := postusecase.NewGetRepliesByPostIDsIncludeDeletedUseCase(postRepository)
	listTopLevelPostsUseCase := postusecase.NewListTopLevelPostsUseCase(postRepository)
	getFeedPostsUseCase := postusecase.NewGetFeedPostsUseCase(postRepository)
	countNewFeedPostsUseCase := postusecase.NewCountNewFeedPostsUseCase(postRepository)
	getRepliesByPostIDsUseCase := postusecase.NewGetRepliesByPostIDsUseCase(postRepository)
	getfavoritePostsByUserIDUseCase := postusecase.NewGetFavoritePostsByUserIDUseCase(postRepository)
	getFollowersTopLevelPostsByUserIDUseCase := postusecase.NewGetFollowersTopLevelPostsByUserIDUseCase(postRepository)

	createFavoriteUseCase := favoriteusecase.NewCreateFavoriteUseCase(favoriteRepository, postRepository)
	deleteFavoriteUseCase := favoriteusecase.NewDeleteFavoriteUseCase(favoriteRepository, postRepository)
	deleteFavoriteByUserIDAndPostIDUseCase := favoriteusecase.NewDeleteFavoriteByUserIDAndPostIDUseCase(favoriteRepository, postRepository)
	getFavoriteByIDUseCase := favoriteusecase.NewGetFavoriteByIDUseCase(favoriteRepository)
	getFavoritesByPostIDUseCase := favoriteusecase.NewGetFavoritesByPostIDUseCase(favoriteRepository)
	getFavoritesByUserIDUseCase := favoriteusecase.NewGetFavoritesByUserIDUseCase(favoriteRepository)
	getFavoriteByUserIDAndPostIDUseCase := favoriteusecase.NewGetFavoriteByUserIDAndPostIDUseCase(favoriteRepository)
	listFavoritesUseCase := favoriteusecase.NewListFavoritesUseCase(favoriteRepository)
	getFavoritesByPostIDsUseCase := favoriteusecase.NewGetFavoritesByPostIDsUseCase(favoriteRepository)

	redisClient, err := infraredis.New()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	revokedTokenRepository := infraredis.NewRedisRevokedTokenRepository(redisClient)
	passwordResetRepository := infraredis.NewRedisPasswordResetRepository(redisClient)
	mailer := infrasmtp.NewSMTPMailer()
	maintenanceRepository := infraredis.NewRedisMaintenanceRepository(redisClient)

	maintenanceFlag := &atomic.Bool{}
	if enabled, err := maintenanceRepository.IsMaintenanceModeEnabled(context.Background()); err != nil {
		logger.Log.Warn().Err(err).Msg("failed to load maintenance mode from redis; defaulting to false")
	} else {
		maintenanceFlag.Store(enabled)
	}

	emailOTPRepository := infraredis.NewRedisEmailOTPRepository(redisClient)
	smtpEmailService := infraemail.NewSMTPEmailService()
	sendEmailOTPUseCase := userusecase.NewSendEmailOTPUseCase(emailOTPRepository, userRepository, smtpEmailService)
	verifyEmailOTPUseCase := userusecase.NewVerifyEmailOTPUseCase(emailOTPRepository)
	createUserUseCase := userusecase.NewCreateUserUseCase(userRepository, profileRepository, emailOTPRepository, txManager)
	refreshUserTokenUseCase := userusecase.NewRefreshUserTokenUseCase(userRepository, revokedTokenRepository)
	refreshAdministratorTokenUseCase := administrator.NewRefreshAdministratorTokenUseCase(administratorRepository, revokedTokenRepository)
	logoutUserUseCase := userusecase.NewLogoutUserUseCase(revokedTokenRepository)
	logoutAdministratorUseCase := administrator.NewLogoutAdministratorUseCase(revokedTokenRepository)
	requestPasswordResetUseCase := userusecase.NewRequestPasswordResetUseCase(userRepository, passwordResetRepository, mailer)
	verifyPasswordResetOTPUseCase := userusecase.NewVerifyPasswordResetOTPUseCase(passwordResetRepository)
	resetPasswordUseCase := userusecase.NewResetPasswordUseCase(userRepository, passwordResetRepository)

	listMediaByPostIDUseCase := mediausecase.NewListMediaByPostIDUseCase(mediaRepository)
	listMediaByPostIDsUseCase := mediausecase.NewListMediaByPostIDsUseCase(mediaRepository)
	listMediaByMessageIDsUseCase := mediausecase.NewListMediaByMessageIDsUseCase(mediaRepository)

	getMessageByIDUseCase := messageusecase.NewGetMessageByIDUseCase(messageRepository)
	sendMessageUseCase := messageusecase.NewSendMessageUseCase(messageRepository, mediaRepository, txManager)
	listMessagesUseCase := messageusecase.NewListMessagesUseCase(messageRepository)
	deleteMessageUseCase := messageusecase.NewDeleteMessageUseCase(messageRepository)
	updateMessageUseCase := messageusecase.NewUpdateMessageUseCase(messageRepository)
	getLastMessagesByRoomIDsUseCase := messageusecase.NewGetLastMessagesByRoomIDsUseCase(messageRepository)
	createRoomUseCase := roomusecase.NewCreateRoomUseCase(roomRepository)
	getRoomUseCase := roomusecase.NewGetRoomUseCase(roomRepository)
	deleteRoomUseCase := roomusecase.NewDeleteRoomUseCase(roomRepository)
	getUserIDsByRoomIDUseCase := roomusecase.NewGetUserIDsByRoomIDUseCase(roomUserRepository)
	listUsersByRoomIDsUseCase := roomusecase.NewListUsersByRoomIDsUseCase(roomUserRepository)
	listMyDMRoomsUseCase := roomusecase.NewListMyDMRoomsUseCase(roomUserRepository)
	getOrCreateDMRoomUseCase := roomusecase.NewGetOrCreateDMRoomUseCase(roomUserRepository)
	addUserToRoomUseCase := roomusecase.NewAddUserToRoomUseCase(roomUserRepository)
	removeUserFromRoomUseCase := roomusecase.NewRemoveUserFromRoomUseCase(roomUserRepository)
	joinRoomUseCase := roomusecase.NewJoinRoomUseCase(roomRepository, roomUserRepository)
	getRoomUserRoleUseCase := roomusecase.NewGetRoomUserRoleUseCase(roomUserRepository)
	setRoomUserRoleUseCase := roomusecase.NewSetRoomUserRoleUseCase(roomUserRepository)
	listRoomMembersWithRolesUseCase := roomusecase.NewListRoomMembersWithRolesUseCase(roomUserRepository)
	markRoomAsReadUseCase := roomusecase.NewMarkRoomAsReadUseCase(roomUserRepository)
	getRoomReadStatusUseCase := roomusecase.NewGetRoomReadStatusUseCase(roomUserRepository, messageRepository)
	getRoomReadStatusBatchUseCase := roomusecase.NewGetRoomReadStatusBatchUseCase(roomUserRepository, messageRepository)
	getMembersUnreadCountsUseCase := roomusecase.NewGetMembersUnreadCountsUseCase(roomUserRepository, messageRepository)

	createCommunityUseCase := communityusecase.NewCreateCommunityUseCase(communityRepository, mediaRepository)
	getCommunityUseCase := communityusecase.NewGetCommunityUseCase(communityRepository)
	updateCommunityUseCase := communityusecase.NewUpdateCommunityUseCase(communityRepository)
	searchCommunityUseCase := communityusecase.NewSearchCommunityUseCase(communityRepository)
	listMyCommunitiesUseCase := communityusecase.NewListMyCommunitiesUseCase(communityRepository)
	listAllCommunitiesUseCase := communityusecase.NewListAllCommunitiesUseCase(communityRepository)
	updateCommunityMembersUseCase := communityusecase.NewUpdateCommunityMembersUseCase(communityRepository, roomUserRepository, txManager)
	promoteToCommunityOwnerUseCase := communityusecase.NewPromoteToCommunityOwnerUseCase(communityRepository, roomUserRepository)
	demoteFromCommunityOwnerUseCase := communityusecase.NewDemoteFromCommunityOwnerUseCase(communityRepository, roomUserRepository)
	isSoleOwnerWithOtherMembersUseCase := communityusecase.NewIsSoleOwnerWithOtherMembersUseCase(communityRepository)
	getRandomCommunitiesUseCase := communityusecase.NewGetRandomCommunitiesUseCase(communityRepository)

	createReportUseCase := reportusecase.NewCreateReportUsecase(reportRepository, systemSettingRepository)
	manageReportUseCase := reportusecase.NewManageReportUsecase(reportRepository)
	manageSystemSettingUseCase := systemsettingsusecase.NewManageSystemSettingUsecase(systemSettingRepository)

	createBlockUseCase := blusecase.NewCreateBlockUseCase(blockRepository, favoriteuserRepository, txManager)
	deleteBlockUseCase := blusecase.NewDeleteBlockerUseCase(blockRepository)
	listBlockersUseCase := blusecase.NewListBlockersUseCase(blockRepository)
	searchBlockersUseCase := blusecase.NewSearchBlockersUseCase(blockRepository)
	getBlockersByUserIDUseCase := blusecase.NewGetBlockersByUserIDUseCase(blockRepository)
	checkBlockRelationUseCase := blusecase.NewCheckBlockRelationUseCase(blockRepository)
	getBlockRelatedUserIDsUseCase := blusecase.NewGetBlockRelatedUserIDsUseCase(blockRepository)

	createFavoriteUserUseCase := fuusecase.NewCreateFavoriteUserUseCase(favoriteuserRepository, blockRepository)
	deleteFavoriteUserUseCase := fuusecase.NewDeleteFavoriteUserUseCase(favoriteuserRepository)
	listFavoriteUsersUseCase := fuusecase.NewListFavoriteUsersUseCase(favoriteuserRepository)
	listFollowersUseCase := fuusecase.NewListFollowersUseCase(favoriteuserRepository)
	searchFavoriteUsersUseCase := fuusecase.NewSearchFavoriteUsersUseCase(favoriteuserRepository)
	getFavoriteUsersByUserIDUseCase := fuusecase.NewGetFavoriteUsersByUserIDUseCase(favoriteuserRepository)
	createInquiryUseCase := inquiryusecase.NewCreateInquiryUsecase(inquiryRepository)
	manageInquiryUseCase := inquiryusecase.NewManageInquiryUsecase(inquiryRepository)

	notificationRepository := mysql.NewMySQLNotificationRepository(database)
	sseBroker := sse.NewBroker()
	notificationPublisher := notificationuc.NewNotificationPublisher(notificationRepository, sseBroker)

	termsRepository := mysql.NewMySQLTermsRepository(database)
	createTermsUseCase := termsusecase.NewCreateTermsUseCase(termsRepository)
	getCurrentTermsUseCase := termsusecase.NewGetCurrentTermsUseCase(termsRepository)
	consentToTermsUseCase := termsusecase.NewConsentToTermsUseCase(termsRepository, userRepository)
	checkConsentUseCase := termsusecase.NewCheckConsentUseCase(termsRepository)
	listTermsUseCase := termsusecase.NewListTermsUseCase(termsRepository)
	listConsentsUseCase := termsusecase.NewListConsentsUseCase(termsRepository)

	announcementRepository := mysql.NewMySQLAnnouncementRepository(database)
	createAnnouncementUseCase := announcementusecase.NewCreateAnnouncementUseCase(announcementRepository, notificationPublisher)
	listAnnouncementsUseCase := announcementusecase.NewListAnnouncementsUseCase(announcementRepository)
	getAnnouncementUseCase := announcementusecase.NewGetAnnouncementUseCase(announcementRepository)
	deleteAnnouncementUseCase := announcementusecase.NewDeleteAnnouncementUseCase(announcementRepository)
	updateAnnouncementUseCase := announcementusecase.NewUpdateAnnouncementUseCase(announcementRepository)
	listNotificationsUseCase := notificationuc.NewListNotificationsUseCase(notificationRepository)
	listNotificationGroupsUseCase := notificationuc.NewListNotificationGroupsUseCase(notificationRepository)
	listNotificationsByActorUseCase := notificationuc.NewListNotificationsByActorUseCase(notificationRepository)
	getNotificationUseCase := notificationuc.NewGetNotificationUseCase(notificationRepository)
	markAsReadUseCase := notificationuc.NewMarkAsReadUseCase(notificationRepository)
	markAllAsReadUseCase := notificationuc.NewMarkAllAsReadUseCase(notificationRepository)
	markAllAsReadByActorUseCase := notificationuc.NewMarkAllAsReadByActorUseCase(notificationRepository)
	countUnreadUseCase := notificationuc.NewCountUnreadUseCase(notificationRepository)
	deleteNotificationsUseCase := notificationuc.NewDeleteNotificationsUseCase(notificationRepository)
	deleteReadNotificationsUseCase := notificationuc.NewDeleteReadNotificationsUseCase(notificationRepository)
	deleteReadNotificationsByActorUseCase := notificationuc.NewDeleteReadNotificationsByActorUseCase(notificationRepository)

	ps := pubsub.New()

	resolver := &graph.Resolver{
		StorageRepository:     storageRepository,
		MaintenanceRepository: maintenanceRepository,
		MaintenanceFlag:       maintenanceFlag,

		UserUseCases: graph.UserUseCases{
			CreateUserUseCase:             createUserUseCase,
			SendEmailOTPUseCase:           sendEmailOTPUseCase,
			VerifyEmailOTPUseCase:         verifyEmailOTPUseCase,
			ListUsersUseCase:              listUsersUseCase,
			DeleteUserUseCase:             deleteUserUseCase,
			UpdateUserUseCase:             updateUserUseCase,
			GetUserByIDUseCase:            getUserByIDUseCase,
			GetUsersByIDsUseCase:          getUsersByIDsUseCase,
			SearchUsersUseCase:            searchUsersUseCase,
			LoginUserUseCase:              loginUserUseCase,
			RefreshUserTokenUseCase:       refreshUserTokenUseCase,
			LogoutUserUseCase:             logoutUserUseCase,
			FreezeUserUseCase:             freezeUserUseCase,
			UnfreezeUserUseCase:           unfreezeUserUseCase,
			RequestPasswordResetUseCase:   requestPasswordResetUseCase,
			VerifyPasswordResetOTPUseCase: verifyPasswordResetOTPUseCase,
			ResetPasswordUseCase:          resetPasswordUseCase,
		},
		SetAvatarUseCase:    setAvatarUseCase,
		DeleteAvatarUseCase: deleteAvatarUseCase,

		GetProfileUseCase:    getProfileUseCase,
		UpdateProfileUseCase: updateProfileUseCase,

		GetAdministratorByIDUseCase:      getAdministratorByIDUseCase,
		CreateAdministratorUseCase:       createAdministratorUseCase,
		CountAdministratorsUseCase:       countAdministratorsUseCase,
		ListAdministratorsUseCase:        listAdministratorsUseCase,
		DeleteAdministratorUseCase:       deleteAdministratorUseCase,
		UpdateAdministratorUseCase:       updateAdministratorUseCase,
		SearchAdministratorsUseCase:      searchAdministratorsUseCase,
		LoginAdministratorUseCase:        loginAdministratorUseCase,
		RefreshAdministratorTokenUseCase: refreshAdministratorTokenUseCase,
		LogoutAdministratorUseCase:       logoutAdministratorUseCase,

		PostUseCases: graph.PostUseCases{
			GetPostByIDUseCase:                       getPostByIDUseCase,
			GetRootPostUseCase:                       getRootPostUseCase,
			GetPostByIDIncludeDeletedUseCase:         getPostByIDIncludeDeletedUseCase,
			CreatePostUseCase:                        createPostUseCase,
			ListPostsUseCase:                         listPostsUseCase,
			DeletePostUseCase:                        deletePostUseCase,
			UpdatePostUseCase:                        updatePostUseCase,
			SearchPostsUseCase:                       searchPostsUseCase,
			ListTopLevelPostsUseCase:                 listTopLevelPostsUseCase,
			GetFeedPostsUseCase:                      getFeedPostsUseCase,
			CountNewFeedPostsUseCase:                 countNewFeedPostsUseCase,
			GetRepliesByIDUseCase:                    getRepliesByIDUseCase,
			GetRepliesByPostIDsIncludeDeletedUseCase: getRepliesByPostIDsIncludeDeletedUseCase,
			GetPostsByUserIDUseCase:                  getPostsByUserIDUseCase,
			GetFavoritePostsByUserIDUseCase:          getfavoritePostsByUserIDUseCase,
			GetFollowersTopLevelPostsByUserIDUseCase: getFollowersTopLevelPostsByUserIDUseCase,
		},

		GetFavoriteByIDUseCase:                 getFavoriteByIDUseCase,
		CreateFavoriteUseCase:                  createFavoriteUseCase,
		DeleteFavoriteUseCase:                  deleteFavoriteUseCase,
		DeleteFavoriteByUserIDAndPostIDUseCase: deleteFavoriteByUserIDAndPostIDUseCase,
		GetFavoriteByUserIDAndPostIDUseCase:    getFavoriteByUserIDAndPostIDUseCase,
		GetFavoritesByPostIDUseCase:            getFavoritesByPostIDUseCase,
		GetFavoritesByUserIDUseCase:            getFavoritesByUserIDUseCase,
		ListFavoritesUseCase:                   listFavoritesUseCase,

		ListMediaByPostIDUseCase: listMediaByPostIDUseCase,

		GetMessageByIDUseCase:           getMessageByIDUseCase,
		SendMessageUseCase:              sendMessageUseCase,
		ListMessagesUseCase:             listMessagesUseCase,
		DeleteMessageUseCase:            deleteMessageUseCase,
		UpdateMessageUseCase:            updateMessageUseCase,
		GetLastMessagesByRoomIDsUseCase: getLastMessagesByRoomIDsUseCase,
		CreateRoomUseCase:               createRoomUseCase,
		GetRoomUseCase:                  getRoomUseCase,
		DeleteRoomUseCase:               deleteRoomUseCase,
		GetUserIDsByRoomIDUseCase:       getUserIDsByRoomIDUseCase,
		ListUsersByRoomIDsUseCase:       listUsersByRoomIDsUseCase,
		ListMyDMRoomsUseCase:            listMyDMRoomsUseCase,
		GetOrCreateDMRoomUseCase:        getOrCreateDMRoomUseCase,
		AddUserToRoomUseCase:            addUserToRoomUseCase,
		RemoveUserFromRoomUseCase:       removeUserFromRoomUseCase,
		JoinRoomUseCase:                 joinRoomUseCase,
		GetRoomUserRoleUseCase:          getRoomUserRoleUseCase,
		SetRoomUserRoleUseCase:          setRoomUserRoleUseCase,
		ListRoomMembersWithRolesUseCase: listRoomMembersWithRolesUseCase,
		MarkRoomAsReadUseCase:           markRoomAsReadUseCase,
		GetRoomReadStatusUseCase:        getRoomReadStatusUseCase,
		GetRoomReadStatusBatchUseCase:   getRoomReadStatusBatchUseCase,
		GetMembersUnreadCountsUseCase:   getMembersUnreadCountsUseCase,

		CreateCommunityUseCase:             createCommunityUseCase,
		GetCommunityUseCase:                getCommunityUseCase,
		UpdateCommunityUseCase:             updateCommunityUseCase,
		UpdateCommunityMembersUseCase:      updateCommunityMembersUseCase,
		SearchCommunityUseCase:             searchCommunityUseCase,
		ListMyCommunitiesUseCase:           listMyCommunitiesUseCase,
		ListAllCommunitiesUseCase:          listAllCommunitiesUseCase,
		PromoteToCommunityOwnerUseCase:     promoteToCommunityOwnerUseCase,
		DemoteFromCommunityOwnerUseCase:    demoteFromCommunityOwnerUseCase,
		IsSoleOwnerWithOtherMembersUseCase: isSoleOwnerWithOtherMembersUseCase,
		GetRandomCommunitiesUseCase:        *getRandomCommunitiesUseCase,

		CreateReportUsecase:        *createReportUseCase,
		ManageReportUsecase:        *manageReportUseCase,
		ManageSystemSettingUsecase: *manageSystemSettingUseCase,

		CreateFavoriteUserUseCase:      createFavoriteUserUseCase,
		DeleteFavoriteUserUseCase:      deleteFavoriteUserUseCase,
		ListFavoriteUsersUseCase:       listFavoriteUsersUseCase,
		ListFollowersUseCase:           listFollowersUseCase,
		SearchFavoriteUsersUseCase:     searchFavoriteUsersUseCase,
		GetFavoriteUserByUserIDUseCase: getFavoriteUsersByUserIDUseCase,

		CreateBlockUseCase:            createBlockUseCase,
		DeleteBlockUseCase:            deleteBlockUseCase,
		ListBlockersUseCase:           listBlockersUseCase,
		SearchBlockersUseCase:         searchBlockersUseCase,
		GetBlockersByUserIDUseCase:    getBlockersByUserIDUseCase,
		CheckBlockRelationUseCase:     checkBlockRelationUseCase,
		GetBlockRelatedUserIDsUseCase: getBlockRelatedUserIDsUseCase,

		CreateInquiryUsecase: *createInquiryUseCase,
		ManageInquiryUsecase: *manageInquiryUseCase,

		CreateAnnouncementUseCase: createAnnouncementUseCase,
		ListAnnouncementsUseCase:  listAnnouncementsUseCase,
		GetAnnouncementUseCase:    getAnnouncementUseCase,
		DeleteAnnouncementUseCase: deleteAnnouncementUseCase,
		UpdateAnnouncementUseCase: updateAnnouncementUseCase,

		CreateTermsUseCase:     createTermsUseCase,
		GetCurrentTermsUseCase: getCurrentTermsUseCase,
		ConsentToTermsUseCase:  consentToTermsUseCase,
		CheckConsentUseCase:    checkConsentUseCase,
		ListTermsUseCase:       listTermsUseCase,
		ListConsentsUseCase:    listConsentsUseCase,

		NotificationPublisher:                 notificationPublisher,
		ListNotificationsUseCase:              listNotificationsUseCase,
		ListNotificationGroupsUseCase:         listNotificationGroupsUseCase,
		ListNotificationsByActorUseCase:       listNotificationsByActorUseCase,
		GetNotificationUseCase:                getNotificationUseCase,
		MarkAsReadUseCase:                     markAsReadUseCase,
		MarkAllAsReadUseCase:                  markAllAsReadUseCase,
		MarkAllAsReadByActorUseCase:           markAllAsReadByActorUseCase,
		CountUnreadUseCase:                    countUnreadUseCase,
		DeleteNotificationsUseCase:            deleteNotificationsUseCase,
		DeleteReadNotificationsUseCase:        deleteReadNotificationsUseCase,
		DeleteReadNotificationsByActorUseCase: deleteReadNotificationsByActorUseCase,
		SSEBroker:                             sseBroker,

		PubSub: ps,
	}

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// ALLOWED_ORIGINS が設定されていれば本番用ホワイトリスト、未設定なら開発用ワイルドカード
	allowedOrigins := allowedOriginsFromEnv(isProd)

	// CORSはJWTより先に登録しないと、401レスポンスにCORSヘッダーが付かずブラウザがブロックする
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"Sec-WebSocket-Protocol",
		},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
	}))
	// RateLimit はIPベースで安価なため、JWT検証（DB/Redis照合あり）より前に置く
	e.Use(authmiddleware.GraphQLRateLimit())
	e.Use(authmiddleware.JWTAuth(revokedTokenRepository, userRepository, passwordResetRepository))
	e.Use(authmiddleware.MaintenanceMode(maintenanceFlag))
	e.Use(authmiddleware.BlockFilter(blockRepository))
	e.Use(authmiddleware.GraphQLAudit())
	e.Use(middleware.BodyLimit("21MB")) // メッセージファイル上限 20MB + マージン
	e.Use(echo.WrapMiddleware(dataloader.Middleware(
		getUsersByIDsUseCase,
		listMediaByPostIDsUseCase,
		listMediaByMessageIDsUseCase,
		getRepliesByPostIDsUseCase,
		getRepliesByPostIDsIncludeDeletedUseCase,
		getFavoritesByPostIDsUseCase,
	)))

	// テスト用エンドポイント
	e.GET("/", func(c echo.Context) error {
		return c.String(200, "test message: Hello from SPACE Server!")
	})

	// GraphQL server with WebSocket transport
	gqlServer := handler.New(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers: resolver,
			},
		),
	)
	gqlServer.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return isOriginAllowed(r.Header.Get("Origin"), allowedOrigins)
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		KeepAlivePingInterval: 10 * time.Second,
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			if _, ok := auth.ClaimsFromContext(ctx); ok {
				return ctx, nil, nil
			}

			authHeader := authHeaderFromInitPayload(initPayload)
			tokenStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(authHeader), "Bearer "))
			if tokenStr == "" {
				return ctx, nil, fmt.Errorf("missing authorization in websocket init payload")
			}

			claims, err := auth.ValidateAndVerifyToken(ctx, tokenStr, revokedTokenRepository, userRepository, passwordResetRepository)
			if err != nil {
				return ctx, nil, err
			}

			ctx = auth.WithClaims(ctx, claims)
			return ctx, nil, nil
		},
	})
	gqlServer.AddTransport(transport.Options{})
	gqlServer.AddTransport(transport.GET{})
	gqlServer.AddTransport(transport.POST{})
	gqlServer.AddTransport(transport.MultipartForm{})
	gqlServer.Use(extension.FixedComplexityLimit(300))

	if !isProd {
		gqlServer.Use(extension.Introspection{})
	}

	// GraphQL エンドポイント (POST: query/mutation, GET: WebSocket subscription)
	gqlHandler := func(c echo.Context) error {
		gqlServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
	e.POST("/query", gqlHandler)
	e.GET("/query", gqlHandler)

	// Playground（開発環境のみ）
	if !isProd {
		e.GET("/playground", func(c echo.Context) error {
			playground.Handler("GraphQL Playground", "/query").
				ServeHTTP(c.Response(), c.Request())
			return nil
		})
	}

	// SSE
	e.GET("/events", sse.NewHandler(sseBroker, notificationRepository, revokedTokenRepository, userRepository, passwordResetRepository))

	schedulePendingTerms(termsRepository, sseBroker)

	go func() {
		if err := e.Start(":8080"); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info().Msg("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Log.Error().Err(err).Msg("server shutdown error")
	}

	if err := database.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("database close error")
	}
	if err := redisClient.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("redis close error")
	}

	logger.Log.Info().Msg("server stopped")
}

// schedulePendingTerms fetches all future-dated terms on startup and sets a one-shot
// timer for each so the SSE broadcast fires exactly when each version becomes effective.
func schedulePendingTerms(termsRepo repository.TermsRepository, broker *sse.Broker) {
	pending, err := termsRepo.FindFuture(context.Background())
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to fetch pending future terms on startup")
		return
	}
	for _, t := range pending {
		version := t.Version
		delay := time.Until(t.EffectiveDate)
		time.AfterFunc(delay, func() {
			broker.Broadcast("terms_updated", map[string]any{"version": version})
			logger.Log.Info().Str("version", version).Msg("scheduled terms now effective, SSE broadcast sent")
		})
		logger.Log.Info().Str("version", version).Dur("delay", delay).Msg("scheduled terms broadcast timer set")
	}
}

// allowedOriginsFromEnv returns the list of allowed CORS/WS origins.
// If ALLOWED_ORIGINS is not set, it falls back to wildcard (dev-friendly).
func allowedOriginsFromEnv(isProd bool) []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	if raw == "" {
		if isProd {
			logger.Log.Fatal().Msg("ALLOWED_ORIGINS must be set in production; refusing to start with wildcard CORS")
		}
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, o := range parts {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// isOriginAllowed checks whether an origin is in the whitelist.
// A whitelist of ["*"] allows every origin.
func isOriginAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}

func authHeaderFromInitPayload(initPayload transport.InitPayload) string {
	for _, key := range []string{"Authorization", "authorization", "authToken", "token", "accessToken"} {
		raw, ok := initPayload[key]
		if !ok {
			continue
		}

		header, ok := raw.(string)
		if !ok {
			continue
		}

		header = strings.TrimSpace(header)
		if header != "" {
			return header
		}
	}

	for _, containerKey := range []string{"headers", "header"} {
		rawContainer, ok := initPayload[containerKey]
		if !ok {
			continue
		}

		switch container := rawContainer.(type) {
		case map[string]interface{}:
			for _, key := range []string{"Authorization", "authorization", "authToken", "token", "accessToken"} {
				raw, ok := container[key]
				if !ok {
					continue
				}
				header, ok := raw.(string)
				if !ok {
					continue
				}
				header = strings.TrimSpace(header)
				if header != "" {
					return header
				}
			}
		case map[string]string:
			for _, key := range []string{"Authorization", "authorization", "authToken", "token", "accessToken"} {
				header := strings.TrimSpace(container[key])
				if header != "" {
					return header
				}
			}
		}
	}

	return ""
}

// bootstrapInitialAdmin creates exactly one initial admin when env vars are provided.
// If INIT_ADMIN_EMAIL/INIT_ADMIN_PASSWORD are empty, this step is skipped.
func bootstrapInitialAdmin(ctx context.Context, adminRepo repository.AdministratorRepository) error {
	name := strings.TrimSpace(os.Getenv("INIT_ADMIN_NAME"))
	email := strings.TrimSpace(os.Getenv("INIT_ADMIN_EMAIL"))
	password := os.Getenv("INIT_ADMIN_PASSWORD")

	if email == "" || password == "" {
		return nil
	}
	if name == "" {
		name = "Initial Admin"
	}

	existing, err := adminRepo.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	admin := &model.Administrator{}
	if err := admin.CreateAdministrator(model.CreateAdministratorParam{
		Name:     name,
		Email:    email,
		Password: password,
	}); err != nil {
		return err
	}

	if err := adminRepo.SaveAdministrator(ctx, admin); err != nil {
		return err
	}

	logger.Log.Info().Str("email", email).Msg("initial administrator created")
	return nil
}
