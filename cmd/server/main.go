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

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Cityboypenguin/SPACE-server/db"
	"github.com/Cityboypenguin/SPACE-server/graph"
	azurerepo "github.com/Cityboypenguin/SPACE-server/infra/azure"
	infracache "github.com/Cityboypenguin/SPACE-server/infra/cache"
	miniorepo "github.com/Cityboypenguin/SPACE-server/infra/minio"
	"github.com/Cityboypenguin/SPACE-server/infra/mysql"
	infraredis "github.com/Cityboypenguin/SPACE-server/infra/redis"
	infrasmtp "github.com/Cityboypenguin/SPACE-server/infra/smtp"
	"github.com/Cityboypenguin/SPACE-server/internal/activityarchive"
	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/async"
	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/internal/config"
	"github.com/Cityboypenguin/SPACE-server/internal/connlimit"
	"github.com/Cityboypenguin/SPACE-server/internal/courseimport"
	"github.com/Cityboypenguin/SPACE-server/internal/dataloader"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/internal/metrics"
	authmiddleware "github.com/Cityboypenguin/SPACE-server/internal/middleware"
	"github.com/Cityboypenguin/SPACE-server/internal/pubsub"
	"github.com/Cityboypenguin/SPACE-server/internal/sse"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/administrator"
	analyticsusecase "github.com/Cityboypenguin/SPACE-server/usecase/analytics"
	announcementusecase "github.com/Cityboypenguin/SPACE-server/usecase/announcement"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	blusecase "github.com/Cityboypenguin/SPACE-server/usecase/block"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	favoriteusecase "github.com/Cityboypenguin/SPACE-server/usecase/favorite"
	fuusecase "github.com/Cityboypenguin/SPACE-server/usecase/favorite_user"
	inquiryusecase "github.com/Cityboypenguin/SPACE-server/usecase/inquiry"
	mediausecase "github.com/Cityboypenguin/SPACE-server/usecase/media"
	messageusecase "github.com/Cityboypenguin/SPACE-server/usecase/message"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
	pollusecase "github.com/Cityboypenguin/SPACE-server/usecase/poll"
	postusecase "github.com/Cityboypenguin/SPACE-server/usecase/post"
	profileusecase "github.com/Cityboypenguin/SPACE-server/usecase/profile"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
	reportusecase "github.com/Cityboypenguin/SPACE-server/usecase/report"
	roomusecase "github.com/Cityboypenguin/SPACE-server/usecase/room"
	sessionusecase "github.com/Cityboypenguin/SPACE-server/usecase/session"
	systemsettingsusecase "github.com/Cityboypenguin/SPACE-server/usecase/system_settings"
	termsusecase "github.com/Cityboypenguin/SPACE-server/usecase/terms"
	userusecase "github.com/Cityboypenguin/SPACE-server/usecase/user"
	usersettingsusecase "github.com/Cityboypenguin/SPACE-server/usecase/user_settings"
	coderws "github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func main() {
	isProd := os.Getenv("APP_ENV") == "production"

	// 起動時に必須設定をまとめて検証し、欠落・危険な設定は即座に落とす。
	// 「その機能を最初に使ったとき」まで問題が潜伏するのを防ぐ。
	if err := config.Validate(isProd); err != nil {
		logger.Log.Fatal().Err(err).Msg("configuration validation failed")
	}

	database, err := mysql.New()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to database")
	}

	if err := db.RunMigrations(database); err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to run migrations")
	}
	logger.Log.Info().Msg("database migrations applied")

	e := echo.New()

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
	userSettingRepository := mysql.NewMySQLUserSettingRepository(database)
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
	// 既存平文メッセージの暗号化はベストエフォートのバックフィル。
	// 一時的な DB 不調で起動をクラッシュループさせないよう、失敗しても続行する
	// （暗号鍵の設定ミスは NewMySQLMessageRepository 側が Fatal で検出する）。
	if encryptedCount, err := messageRepository.EncryptPlaintextMessages(context.Background(), 500); err != nil {
		logger.Log.Error().Err(err).Msg("failed to encrypt existing message history; will retry on next startup")
	} else if encryptedCount > 0 {
		logger.Log.Info().Int("count", encryptedCount).Msg("encrypted existing message history")
	}
	mediaRepository := mysql.NewMySQLMediaRepository(database)
	roomRepository := mysql.NewMySQLRoomRepository(database)
	roomUserRepository := mysql.NewMySQLRoomUserRepository(database)
	communityRepository := mysql.NewMySQLCommunityRepository(database)
	courseRepository := mysql.NewMySQLCourseRepository(database)
	ps := pubsub.New()
	courseImportTracker := courseimport.NewTracker(func(status courseimport.Status) {
		ps.Publish(graph.CourseImportStatusTopic, status)
	})
	timetableRepository := mysql.NewMySQLTimetableRepository(database)
	roomAnonymousIdentityRepository := mysql.NewMySQLRoomAnonymousIdentityRepository(database)
	courseRoomReadRepository := mysql.NewMySQLCourseRoomReadRepository(database)
	questionRepository, err := mysql.NewMySQLQuestionRepository(database)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to initialize question repository")
	}
	answerRepository, err := mysql.NewMySQLAnswerRepository(database)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to initialize answer repository")
	}
	pollRepository := mysql.NewMySQLPollRepository(database)

	var storageRepository repository.StorageRepository
	var privateStorageRepository repository.PrivateStorageRepository
	if os.Getenv("STORAGE_PROVIDER") == "azure" {
		storage, storageErr := azurerepo.New()
		err = storageErr
		if err != nil {
			logger.Log.Fatal().Err(err).Msg("failed to connect to azure blob storage")
		}
		storageRepository = storage
		privateStorageRepository = storage
	} else {
		storage, storageErr := miniorepo.New()
		err = storageErr
		if err != nil {
			logger.Log.Fatal().Err(err).Msg("failed to connect to minio")
		}
		storageRepository = storage
		privateStorageRepository = storage
	}
	activityArchiveRepository := mysql.NewMySQLActivityArchiveRepository(database)
	activityArchiver, err := activityarchive.New(activityArchiveRepository, privateStorageRepository, config.ActivityArchiveHMACKey(isProd))
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to initialize activity archive")
	}
	activityArchiveCtx, stopActivityArchive := context.WithCancel(context.Background())
	activityArchiveDone := make(chan struct{})

	listUsersUseCase := userusecase.NewListUsersUseCase(userRepository)
	deleteUserUseCase := userusecase.NewDeleteUserUseCase(userRepository, postRepository, roomRepository, roomUserRepository, txManager)
	updateUserUseCase := userusecase.NewUpdateUserUseCase(userRepository)
	getUserByIDUseCase := userusecase.NewGetUserByIDUseCase(userRepository)
	getUsersByIDsUseCase := userusecase.NewGetUsersByIDsUseCase(userRepository)
	searchUsersUseCase := userusecase.NewSearchUsersUseCase(userRepository)
	// 連絡先まで返す取得（本人・管理者向け）。表示系は上の3つを使う。
	getUserAccountByIDUseCase := userusecase.NewGetUserAccountByIDUseCase(userRepository)
	getUserAccountsByIDsUseCase := userusecase.NewGetUserAccountsByIDsUseCase(userRepository)
	searchUserAccountsUseCase := userusecase.NewSearchUserAccountsUseCase(userRepository)
	suggestUsersUseCase := userusecase.NewSuggestUsersUseCase(userRepository)
	loginUserUseCase := userusecase.NewLoginUserUseCase(userRepository)
	freezeUserUseCase := userusecase.NewFreezeUserUseCase(userRepository)
	unfreezeUserUseCase := userusecase.NewUnfreezeUserUseCase(userRepository)
	getProfileUseCase := profileusecase.NewGetProfileUseCase(profileRepository)
	updateProfileUseCase := profileusecase.NewUpdateProfileUseCase(profileRepository)
	updateMyProfileUseCase := profileusecase.NewUpdateMyProfileUseCase(userRepository, profileRepository, txManager)
	setAvatarUseCase := profileusecase.NewSetAvatarUseCase(profileRepository, mediaRepository, txManager)
	deleteAvatarUseCase := profileusecase.NewDeleteAvatarUseCase(profileRepository)

	createAdministratorUseCase := administrator.NewCreateAdministratorUseCase(administratorRepository, txManager)
	countAdministratorsUseCase := administrator.NewCountAdministratorsUseCase(administratorRepository)
	getAdministratorByIDUseCase := administrator.NewGetAdministratorByIDUseCase(administratorRepository)
	listAdministratorsUseCase := administrator.NewListAdministratorsUseCase(administratorRepository)
	deleteAdministratorUseCase := administrator.NewDeleteAdministratorUseCase(administratorRepository, txManager)
	updateAdministratorUseCase := administrator.NewUpdateAdministratorUseCase(administratorRepository)
	searchAdministratorsUseCase := administrator.NewSearchAdministratorsUseCase(administratorRepository)
	loginAdministratorUseCase := administrator.NewLoginAdministratorUseCase(administratorRepository)

	// createPostUseCase / updatePostUseCase は通知発行のため notificationPublisher に依存する。
	// publisher 構築後（下方）に生成する。
	deletePostUseCase := postusecase.NewDeletePostUseCase(postRepository)
	getPostByIDUseCase := postusecase.NewGetPostByIDUseCase(postRepository)
	getPostsByIDsUseCase := postusecase.NewGetPostsByIDsUseCase(postRepository)
	getRootPostUseCase := postusecase.NewGetRootPostUseCase(postRepository)
	getPostByIDIncludeDeletedUseCase := postusecase.NewGetPostByIDIncludeDeletedUseCase(postRepository)
	listPostsUseCase := postusecase.NewListPostsUseCase(postRepository)
	searchPostsUseCase := postusecase.NewSearchPostsUseCase(postRepository)
	searchPostsByHashtagUseCase := postusecase.NewSearchPostsByHashtagUseCase(postRepository)
	popularHashtagsUseCase := postusecase.NewPopularHashtagsUseCase(postRepository)
	suggestHashtagsUseCase := postusecase.NewSuggestHashtagsUseCase(postRepository)
	getPostsByUserIDUseCase := postusecase.NewGetPostsByUserIDUseCase(postRepository)
	getRepliesByIDUseCase := postusecase.NewGetRepliesByIDUseCase(postRepository)
	getRepliesByPostIDsIncludeDeletedUseCase := postusecase.NewGetRepliesByPostIDsIncludeDeletedUseCase(postRepository)
	listTopLevelPostsUseCase := postusecase.NewListTopLevelPostsUseCase(postRepository)
	getFeedPostsUseCase := postusecase.NewGetFeedPostsUseCase(postRepository)
	countNewFeedPostsUseCase := postusecase.NewCountNewFeedPostsUseCase(postRepository)
	getRepliesByPostIDsUseCase := postusecase.NewGetRepliesByPostIDsUseCase(postRepository)
	getfavoritePostsByUserIDUseCase := postusecase.NewGetFavoritePostsByUserIDUseCase(postRepository)
	getFollowersTopLevelPostsByUserIDUseCase := postusecase.NewGetFollowersTopLevelPostsByUserIDUseCase(postRepository)

	// createFavoriteUseCase も notificationPublisher に依存するため publisher 構築後に生成する。
	deleteFavoriteUseCase := favoriteusecase.NewDeleteFavoriteUseCase(favoriteRepository, postRepository)
	deleteFavoriteByUserIDAndPostIDUseCase := favoriteusecase.NewDeleteFavoriteByUserIDAndPostIDUseCase(favoriteRepository, postRepository)
	getFavoriteByIDUseCase := favoriteusecase.NewGetFavoriteByIDUseCase(favoriteRepository)
	getFavoritesByPostIDUseCase := favoriteusecase.NewGetFavoritesByPostIDUseCase(favoriteRepository)
	getFavoritesByUserIDUseCase := favoriteusecase.NewGetFavoritesByUserIDUseCase(favoriteRepository)
	getFavoriteByUserIDAndPostIDUseCase := favoriteusecase.NewGetFavoriteByUserIDAndPostIDUseCase(favoriteRepository)
	getFavoritesByPostIDsUseCase := favoriteusecase.NewGetFavoritesByPostIDsUseCase(favoriteRepository)

	redisClient, err := infraredis.New()
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	revokedTokenRepository := infraredis.NewRedisRevokedTokenRepository(redisClient)
	passwordResetRepository := infraredis.NewRedisPasswordResetRepository(redisClient)
	// SSE(/events) の接続チケット。Redis に置くのは寿命(30秒)の管理を任せられるのと、
	// 将来インスタンスを増やしたときに「発行した台と接続先の台が違う」でも引き換えが
	// 通るようにするため（docs/realtime-scaling.md）。
	sseTicketRepository := infraredis.NewRedisSSETicketRepository(redisClient)
	mailer := infrasmtp.NewSMTPMailer()
	maintenanceRepository := infraredis.NewRedisMaintenanceRepository(redisClient)

	maintenanceFlag := &atomic.Bool{}
	if enabled, err := maintenanceRepository.IsMaintenanceModeEnabled(context.Background()); err != nil {
		logger.Log.Warn().Err(err).Msg("failed to load maintenance mode from redis; defaulting to false")
	} else {
		maintenanceFlag.Store(enabled)
	}

	emailOTPRepository := infraredis.NewRedisEmailOTPRepository(redisClient)
	sendEmailOTPUseCase := userusecase.NewSendEmailOTPUseCase(emailOTPRepository, userRepository, mailer)
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
	listMediaByQuestionIDsUseCase := mediausecase.NewListMediaByQuestionIDsUseCase(mediaRepository)
	listMediaByAnswerIDsUseCase := mediausecase.NewListMediaByAnswerIDsUseCase(mediaRepository)

	getMessageByIDUseCase := messageusecase.NewGetMessageByIDUseCase(messageRepository)
	getMessagesByIDsUseCase := messageusecase.NewGetMessagesByIDsUseCase(messageRepository)
	// messageRepository は MessageReader / MessageWriter / MessageReadModel /
	// MessageMentionStore / MessageUnreadCounter の合成実装。各ユースケースには必要な
	// 口だけを渡す。特に書き込みの口 (MessageWriter) を渡す先は下の
	// chatusecase.NewMessageWriters ただ1箇所に絞ること（理由は
	// repository.MessageWriter / usecase/chat/writers.go のコメント参照）。
	listMessagesAroundUseCase := messageusecase.NewListMessagesAroundUseCase(messageRepository, messageRepository)
	reportMediaDimensionsUseCase := mediausecase.NewReportDimensionsUseCase(mediaRepository)
	listImagesMissingDimensionsUseCase := mediausecase.NewListImagesMissingDimensionsUseCase(mediaRepository)
	listMessagesUseCase := messageusecase.NewListMessagesUseCase(messageRepository)
	// メッセージの保存処理（送信・編集・削除）は usecase/chat/internal/messagestore に
	// あり、ここから直接は組み立てられない。認可を通さず保存を叩ける口を作らないための
	// 構造なので、束ごと受け取ってチャットサービスへ渡す（usecase/chat/writers.go 参照）。
	chatMessageWriters := chatusecase.NewMessageWriters(messageRepository, messageRepository, messageRepository, mediaRepository, txManager)
	resolveMessageMentionsUseCase := messageusecase.NewResolveMentionsUseCase(userRepository, roomRepository, roomUserRepository, blockRepository)
	listMessageMentionsUseCase := messageusecase.NewListMentionsByMessageIDsUseCase(messageRepository)
	getLastMessagesByRoomIDsUseCase := messageusecase.NewGetLastMessagesByRoomIDsUseCase(messageRepository)
	getRoomUseCase := roomusecase.NewGetRoomUseCase(roomRepository)
	getUserIDsByRoomIDUseCase := roomusecase.NewGetUserIDsByRoomIDUseCase(roomUserRepository)
	listUsersByRoomIDsUseCase := roomusecase.NewListUsersByRoomIDsUseCase(roomUserRepository)
	searchRoomUsersUseCase := roomusecase.NewSearchRoomUsersUseCase(roomUserRepository)
	countUsersByRoomIDsUseCase := roomusecase.NewCountUsersByRoomIDsUseCase(roomUserRepository)
	listJoinedRoomIDsUseCase := roomusecase.NewListJoinedRoomIDsUseCase(roomUserRepository)
	listMyDMRoomsUseCase := roomusecase.NewListMyDMRoomsUseCase(roomUserRepository)
	getOrCreateDMRoomUseCase := roomusecase.NewGetOrCreateDMRoomUseCase(roomUserRepository)
	leaveCommunityUseCase := roomusecase.NewLeaveCommunityUseCase(roomRepository, roomUserRepository, txManager)
	deleteOrphanedDMUseCase := roomusecase.NewDeleteOrphanedDMUseCase(roomRepository, roomUserRepository, txManager)
	joinRoomUseCase := roomusecase.NewJoinRoomUseCase(roomRepository, roomUserRepository)
	getRoomUserRoleUseCase := roomusecase.NewGetRoomUserRoleUseCase(roomUserRepository)
	setRoomUserRoleUseCase := roomusecase.NewSetRoomUserRoleUseCase(roomUserRepository)
	listRoomMembersWithRolesUseCase := roomusecase.NewListRoomMembersWithRolesUseCase(roomUserRepository)
	listRoomMembersWithRolesPageUseCase := roomusecase.NewListRoomMembersWithRolesPageUseCase(roomUserRepository)
	// 既読位置はメッセージIDで持つので、既読を打つ側は「ルームの最新メッセージID」を
	// 引ける messageRepository も要る。
	markRoomAsReadUseCase := roomusecase.NewMarkRoomAsReadUseCase(roomUserRepository, messageRepository)
	getRoomReadStatusUseCase := roomusecase.NewGetRoomReadStatusUseCase(roomUserRepository, messageRepository)
	markCourseRoomAsReadUseCase := roomusecase.NewMarkCourseRoomAsReadUseCase(courseRoomReadRepository, messageRepository)
	// 授業ルームの学期・履修判定。チャットサービスが送信・編集・削除で使う。
	checkRoomWritableUseCase := courseusecase.NewCheckRoomWritableUseCase(courseRepository, systemSettingRepository, timetableRepository)
	getOrCreateAnonymousIdentityUseCase := anonusecase.NewGetOrCreateAnonymousIdentityUseCase(roomAnonymousIdentityRepository)
	// 採番しない読み取り専用の口。授業ルームの返信通知の文言（匿名NNN）に使う。
	getAnonymousIdentityUseCase := anonusecase.NewGetAnonymousIdentityUseCase(roomAnonymousIdentityRepository)
	getCourseRoomReadStatusUseCase := roomusecase.NewGetCourseRoomReadStatusUseCase(courseRoomReadRepository, messageRepository, courseRepository, timetableRepository)
	getRoomReadStatusBatchUseCase := roomusecase.NewGetRoomReadStatusBatchUseCase(roomUserRepository, messageRepository)
	// 授業ルームの更新通知の宛先（履修者）。未読数は数えず、IDだけを引く軽い経路
	// （usecase/room/get_course_registrant_ids.go のコメント参照）。
	getCourseRegistrantIDsUseCase := roomusecase.NewGetCourseRegistrantIDsUseCase(timetableRepository)
	countUnreadByRoomTypeUseCase := roomusecase.NewCountUnreadByRoomTypeUseCase(messageRepository)

	// コミュニティ系ユースケースの生成は graph.NewCommunityUseCases に集約。

	createReportUseCase := reportusecase.NewCreateReportUsecase(reportRepository, systemSettingRepository)
	manageReportUseCase := reportusecase.NewManageReportUsecase(reportRepository)
	manageSystemSettingUseCase := systemsettingsusecase.NewManageSystemSettingUsecase(systemSettingRepository)
	manageUserSettingUseCase := usersettingsusecase.NewManageUserSettingUsecase(userSettingRepository)

	cachedAnalyticsRepo := infracache.NewCachedAnalyticsRepository(
		mysql.NewMySQLAnalyticsRepository(database),
		5*time.Minute,
		2*time.Minute,
	)
	analyticsRepository := cachedAnalyticsRepo
	getAnalyticsUseCase := analyticsusecase.NewGetAnalyticsUseCase(analyticsRepository)
	getCommunityAnalyticsUseCase := analyticsusecase.NewGetCommunityAnalyticsUseCase(analyticsRepository)
	getTimeSeriesUseCase := analyticsusecase.NewGetTimeSeriesUseCase(analyticsRepository)

	sessionRepository := mysql.NewMySQLSessionRepository(database)
	recordSessionUseCase := sessionusecase.NewRecordSessionUseCase(sessionRepository)

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

	// 通知発行を伴うユースケースは publisher を注入して生成する。
	createPostUseCase := postusecase.NewCreatePostUseCase(postRepository, mediaRepository, userRepository, blockRepository, txManager, notificationPublisher)
	updatePostUseCase := postusecase.NewUpdatePostUseCase(postRepository, mediaRepository, userRepository, blockRepository, txManager, notificationPublisher)
	listPostMentionsUseCase := postusecase.NewListMentionsByPostIDsUseCase(postRepository)

	// DataLoader からしか使わないバッチ取得の口。リゾルバは dataloader.For(ctx) 経由で
	// 触るので Resolver には持たせない（単体取得と二重に持つと、片方だけ使う
	// リゾルバが残って N+1 が戻る）。
	getRoomsByIDsUseCase := roomusecase.NewGetRoomsByIDsUseCase(roomRepository)
	getQuestionsByIDsUseCase := questionusecase.NewGetQuestionsByIDsUseCase(questionRepository)
	getAnswersByIDsUseCase := answerusecase.NewGetAnswersByIDsUseCase(answerRepository)
	listAnswerPagesByQuestionIDsUseCase := answerusecase.NewListAnswerPagesByQuestionIDsUseCase(answerRepository)
	listPollOptionResultsByPollIDsUseCase := pollusecase.NewListPollOptionResultsByPollIDsUseCase(pollRepository)
	countPollVotersByPollIDsUseCase := pollusecase.NewCountPollVotersByPollIDsUseCase(pollRepository)
	getAnonymousIdentitiesUseCase := anonusecase.NewGetAnonymousIdentitiesUseCase(roomAnonymousIdentityRepository)
	createFavoriteUseCase := favoriteusecase.NewCreateFavoriteUseCase(favoriteRepository, postRepository, notificationPublisher)

	termsRepository := mysql.NewMySQLTermsRepository(database)
	createTermsUseCase := termsusecase.NewCreateTermsUseCase(termsRepository)
	getCurrentTermsUseCase := termsusecase.NewGetCurrentTermsUseCase(termsRepository)
	consentToTermsUseCase := termsusecase.NewConsentToTermsUseCase(termsRepository, userRepository)
	checkConsentUseCase := termsusecase.NewCheckConsentUseCase(termsRepository)
	listTermsUseCase := termsusecase.NewListTermsUseCase(termsRepository)
	listConsentsUseCase := termsusecase.NewListConsentsUseCase(termsRepository)
	termsBroadcastScheduler := termsusecase.NewBroadcastScheduler(termsRepository, sseBroker)

	// リクエストの応答を待たせずに走らせる処理（チャット配信・お知らせ通知・活動記録）の
	// 実行口。流儀は internal/async.Runner の1つだけに揃えてあり、component 名だけが違う。
	// ここでまとめて作るのは、停止時に「全部待つ」を1箇所（asyncRunners）で書くため。
	chatEventAsyncRunner := chatusecase.NewAsyncRunner()
	announcementAsyncRunner := async.NewRunner("announcement")
	userActivityAsyncRunner := async.NewRunner("user_activity")
	asyncRunners := []*async.Runner{chatEventAsyncRunner, announcementAsyncRunner, userActivityAsyncRunner}

	// 認証済みリクエストの活動記録。毎リクエスト DB へ書かないよう、ユーザーごとに
	// 間引いてから Runner へ渡す（internal/middleware/user_activity.go 参照）。
	userActivityRecorder := authmiddleware.NewUserActivityRecorder(userRepository, userActivityAsyncRunner)

	announcementRepository := mysql.NewMySQLAnnouncementRepository(database)
	createAnnouncementUseCase := announcementusecase.NewCreateAnnouncementUseCase(announcementRepository, notificationPublisher, announcementAsyncRunner)
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
	issueStreamTicketUseCase := notificationuc.NewIssueStreamTicketUseCase(sseTicketRepository)

	// チャットの配線は publisher → 権限判定 → 各サービス → resolver の一方向。
	// 以前は配信アダプタが *Resolver をまるごと持っていたため「resolver を作ってから
	// サービスを差し込む」相互参照になっていた。アダプタが必要な依存だけを受け取る形に
	// したので、resolver を組み立てる前に全部そろう。
	//
	// 配信のうち順序も応答時間も要らないぶん（room_changed の SSE と各種通知）だけを
	// リクエストの外へ出す。購読中の画面へ流す PubSub は順序が崩れるとチャット本体の
	// 並びが壊れるので、アダプタの中で同期のまま残してある（どちらがどちらかは
	// graph/chat_events.go と usecase/chat/async_events.go のコメント参照）。
	chatEventPublisher := graph.NewChatEventPublisher(graph.ChatEventPublisherDeps{
		PubSub:                         ps,
		SSEBroker:                      sseBroker,
		NotificationPublisher:          notificationPublisher,
		CourseRegistrantIDs:            getCourseRegistrantIDsUseCase,
		Async:                          chatEventAsyncRunner,
		GetMessage:                     getMessageByIDUseCase,
		GetAnonymousIdentity:           getAnonymousIdentityUseCase,
		MarkNotificationsAsReadByActor: markAllAsReadByActorUseCase,
	})

	// 権限判定は1つだけ作り、送信・一覧・既読の各サービスで共有する
	// （判定の実体を複数持たせないため。usecase/chat のパッケージコメント参照）。
	chatAccessPolicy := chatusecase.NewAccessPolicy(chatusecase.AccessPolicyDeps{
		GetRoom:            getRoomUseCase,
		GetRoomMemberIDs:   getUserIDsByRoomIDUseCase,
		CheckRoomWritable:  checkRoomWritableUseCase,
		CheckBlockRelation: checkBlockRelationUseCase,
	})
	chatCommandService := chatusecase.NewMessageCommandService(chatusecase.MessageCommandDeps{
		Access:                       chatAccessPolicy,
		GetRoom:                      getRoomUseCase,
		GetRoomUserRole:              getRoomUserRoleUseCase,
		GetMessage:                   getMessageByIDUseCase,
		Writers:                      chatMessageWriters,
		ResolveMentions:              resolveMessageMentionsUseCase,
		ListMentions:                 listMessageMentionsUseCase,
		ListMessageMedia:             listMediaByMessageIDsUseCase,
		GetOrCreateAnonymousIdentity: getOrCreateAnonymousIdentityUseCase,
		Events:                       chatEventPublisher,
	})
	chatQueryService := chatusecase.NewMessageQueryService(chatusecase.MessageQueryDeps{
		Access:             chatAccessPolicy,
		ListMessages:       listMessagesUseCase,
		ListMessagesAround: listMessagesAroundUseCase,
	})
	chatReadService := chatusecase.NewReadReceiptService(chatusecase.ReadReceiptDeps{
		Access:                  chatAccessPolicy,
		GetRoom:                 getRoomUseCase,
		GetRoomMemberIDs:        getUserIDsByRoomIDUseCase,
		MarkRoomAsRead:          markRoomAsReadUseCase,
		MarkCourseRoomAsRead:    markCourseRoomAsReadUseCase,
		GetRoomReadStatus:       getRoomReadStatusUseCase,
		GetCourseRoomReadStatus: getCourseRoomReadStatusUseCase,
		Events:                  chatEventPublisher,
	})

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
			GetUserAccountByIDUseCase:     getUserAccountByIDUseCase,
			GetUserAccountsByIDsUseCase:   getUserAccountsByIDsUseCase,
			SearchUserAccountsUseCase:     searchUserAccountsUseCase,
			SuggestUsersUseCase:           suggestUsersUseCase,
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

		GetProfileUseCase:      getProfileUseCase,
		UpdateProfileUseCase:   updateProfileUseCase,
		UpdateMyProfileUseCase: updateMyProfileUseCase,

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
			GetPostsByIDsUseCase:                     getPostsByIDsUseCase,
			GetRootPostUseCase:                       getRootPostUseCase,
			GetPostByIDIncludeDeletedUseCase:         getPostByIDIncludeDeletedUseCase,
			CreatePostUseCase:                        createPostUseCase,
			ListPostsUseCase:                         listPostsUseCase,
			DeletePostUseCase:                        deletePostUseCase,
			UpdatePostUseCase:                        updatePostUseCase,
			SearchPostsUseCase:                       searchPostsUseCase,
			SearchPostsByHashtagUseCase:              searchPostsByHashtagUseCase,
			PopularHashtagsUseCase:                   popularHashtagsUseCase,
			SuggestHashtagsUseCase:                   suggestHashtagsUseCase,
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

		ListMediaByPostIDUseCase:           listMediaByPostIDUseCase,
		ReportMediaDimensionsUseCase:       reportMediaDimensionsUseCase,
		ListImagesMissingDimensionsUseCase: listImagesMissingDimensionsUseCase,

		MessageRoomUseCases: graph.MessageRoomUseCases{
			GetMessageByIDUseCase:               getMessageByIDUseCase,
			GetLastMessagesByRoomIDsUseCase:     getLastMessagesByRoomIDsUseCase,
			GetRoomUseCase:                      getRoomUseCase,
			GetUserIDsByRoomIDUseCase:           getUserIDsByRoomIDUseCase,
			ListUsersByRoomIDsUseCase:           listUsersByRoomIDsUseCase,
			SearchRoomUsersUseCase:              searchRoomUsersUseCase,
			CountUsersByRoomIDsUseCase:          countUsersByRoomIDsUseCase,
			ListJoinedRoomIDsUseCase:            listJoinedRoomIDsUseCase,
			ListMyDMRoomsUseCase:                listMyDMRoomsUseCase,
			GetOrCreateDMRoomUseCase:            getOrCreateDMRoomUseCase,
			LeaveCommunityUseCase:               leaveCommunityUseCase,
			DeleteOrphanedDMUseCase:             deleteOrphanedDMUseCase,
			JoinRoomUseCase:                     joinRoomUseCase,
			GetRoomUserRoleUseCase:              getRoomUserRoleUseCase,
			SetRoomUserRoleUseCase:              setRoomUserRoleUseCase,
			ListRoomMembersWithRolesUseCase:     listRoomMembersWithRolesUseCase,
			ListRoomMembersWithRolesPageUseCase: listRoomMembersWithRolesPageUseCase,
			GetRoomReadStatusBatchUseCase:       getRoomReadStatusBatchUseCase,
			CountUnreadByRoomTypeUseCase:        countUnreadByRoomTypeUseCase,
		},

		CommunityUseCases: graph.NewCommunityUseCases(communityRepository, roomUserRepository, txManager),
		CourseUseCases:    graph.NewCourseUseCases(courseRepository, timetableRepository, systemSettingRepository, roomAnonymousIdentityRepository, userSettingRepository, roomRepository, blockRepository, messageRepository),
		QuestionUseCases:  graph.NewQuestionUseCases(questionRepository, answerRepository, mediaRepository, txManager, courseRepository, systemSettingRepository, timetableRepository, roomAnonymousIdentityRepository),
		PollUseCases:      graph.NewPollUseCases(pollRepository, courseRepository, systemSettingRepository, timetableRepository, roomAnonymousIdentityRepository),

		CreateReportUsecase:          *createReportUseCase,
		ManageReportUsecase:          *manageReportUseCase,
		ManageSystemSettingUsecase:   *manageSystemSettingUseCase,
		ManageUserSettingUsecase:     *manageUserSettingUseCase,
		GetAnalyticsUseCase:          getAnalyticsUseCase,
		GetCommunityAnalyticsUseCase: getCommunityAnalyticsUseCase,
		GetTimeSeriesUseCase:         getTimeSeriesUseCase,
		RecordSessionUseCase:         recordSessionUseCase,
		InvalidateAnalyticsSummary:   cachedAnalyticsRepo.InvalidateSummary,

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

		CreateTermsUseCase:      createTermsUseCase,
		TermsBroadcastScheduler: termsBroadcastScheduler,
		GetCurrentTermsUseCase:  getCurrentTermsUseCase,
		ConsentToTermsUseCase:   consentToTermsUseCase,
		CheckConsentUseCase:     checkConsentUseCase,
		ListTermsUseCase:        listTermsUseCase,
		ListConsentsUseCase:     listConsentsUseCase,

		NotificationUseCases: graph.NotificationUseCases{
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
			IssueStreamTicketUseCase:              issueStreamTicketUseCase,
		},
		SSEBroker: sseBroker,

		PubSub: ps,

		CourseImportTracker: courseImportTracker,

		ChatUseCases: graph.ChatUseCases{
			ChatAccess:   chatAccessPolicy,
			ChatCommands: chatCommandService,
			ChatQueries:  chatQueryService,
			ChatReads:    chatReadService,
		},
	}

	// middleware
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(authmiddleware.RequestTimeout(30 * time.Second))

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
	e.Use(authmiddleware.MetricsMiddleware())
	e.Use(authmiddleware.JWTAuth(revokedTokenRepository, userRepository, administratorRepository, userActivityRecorder))
	e.Use(authmiddleware.MaintenanceMode(maintenanceFlag))
	e.Use(authmiddleware.BlockFilter(blockRepository))
	e.Use(authmiddleware.GraphQLAudit())
	e.Use(middleware.BodyLimit("21MB")) // メッセージファイル上限 20MB + マージン
	// DataLoader はリクエストごとに作り直す（キャッシュがリクエスト内でだけ正しいため）。
	// 渡す口は名前付きフィールドで指定する。同じ形のインターフェースが並ぶので、
	// 位置引数だと取り違えてもコンパイルが通ってしまう。
	e.Use(echo.WrapMiddleware(dataloader.Middleware(dataloader.UseCases{
		GetUsersByIDs:                  getUsersByIDsUseCase,
		GetPostsByIDs:                  getPostsByIDsUseCase,
		ListMediaByPostIDs:             listMediaByPostIDsUseCase,
		ListMediaByMessageIDs:          listMediaByMessageIDsUseCase,
		ListMediaByQuestionIDs:         listMediaByQuestionIDsUseCase,
		ListMediaByAnswerIDs:           listMediaByAnswerIDsUseCase,
		GetRepliesByPostIDs:            getRepliesByPostIDsUseCase,
		GetRepliesByPostIDsIncludeDel:  getRepliesByPostIDsIncludeDeletedUseCase,
		GetFavoritesByPostIDs:          getFavoritesByPostIDsUseCase,
		GetMessagesByIDs:               getMessagesByIDsUseCase,
		ListMentionsByPostIDs:          listPostMentionsUseCase,
		ListMentionsByMessageIDs:       listMessageMentionsUseCase,
		GetRoomsByIDs:                  getRoomsByIDsUseCase,
		GetQuestionsByIDs:              getQuestionsByIDsUseCase,
		GetAnswersByIDs:                getAnswersByIDsUseCase,
		ListAnswerPagesByQuestionIDs:   listAnswerPagesByQuestionIDsUseCase,
		ListPollOptionResultsByPollIDs: listPollOptionResultsByPollIDsUseCase,
		CountPollVotersByPollIDs:       countPollVotersByPollIDsUseCase,
		GetAnonymousIdentities:         getAnonymousIdentitiesUseCase,
	})))

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
	// コンテキストキー：InitFunc成功・ユーザーIDをCloseFunc側で参照するために使う
	type wsConnectedKey struct{}
	type wsUserIDKey struct{}

	wsLimiter := connlimit.NewWSLimiter()

	gqlServer.AddTransport(transport.Websocket{
		// gqlgen v0.17.95 で WebSocket 実装が gorilla/websocket から coder/websocket に
		// 置き換わり、Upgrader フィールドが無くなった。オリジン検証は
		// WebsocketImplementation を差し替えて従来どおり isOriginAllowed で行う。
		Implementation:        newOriginCheckingWebsocketImplementation(allowedOrigins),
		KeepAlivePingInterval: 10 * time.Second,
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			var userID int64
			if claims, ok := auth.ClaimsFromContext(ctx); ok {
				userID = claims.ID
			} else {
				authHeader := authHeaderFromInitPayload(initPayload)
				tokenStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(authHeader), "Bearer "))
				if tokenStr == "" {
					return ctx, nil, fmt.Errorf("missing authorization in websocket init payload")
				}
				claims, err := auth.ValidateAndVerifyToken(ctx, tokenStr, revokedTokenRepository, userRepository, administratorRepository)
				if err != nil {
					return ctx, nil, err
				}
				ctx = auth.WithClaims(ctx, claims)
				userID = claims.ID
			}

			if err := wsLimiter.Acquire(userID); err != nil {
				return ctx, nil, fmt.Errorf("too many WebSocket connections")
			}
			metrics.Global.IncWSConnections()
			ctx = context.WithValue(ctx, wsConnectedKey{}, true)
			ctx = context.WithValue(ctx, wsUserIDKey{}, userID)
			return ctx, nil, nil
		},
		CloseFunc: func(ctx context.Context, _ int) {
			if ctx.Value(wsConnectedKey{}) == true {
				metrics.Global.DecWSConnections()
				if userID, ok := ctx.Value(wsUserIDKey{}).(int64); ok {
					wsLimiter.Release(userID)
				}
			}
		},
	})
	gqlServer.AddTransport(transport.Options{})
	gqlServer.AddTransport(transport.GET{})
	gqlServer.AddTransport(transport.POST{})
	gqlServer.AddTransport(transport.MultipartForm{})
	gqlServer.Use(extension.FixedComplexityLimit(300))

	// エラーメッセージ本文ではなく extensions.code を API 契約にする。
	// クライアントは文言ではなくコードで分岐できるので、文言変更で壊れない。
	gqlServer.SetErrorPresenter(func(ctx context.Context, e error) *gqlerror.Error {
		gqlErr := graphql.DefaultErrorPresenter(ctx, e)
		if gqlErr.Extensions == nil {
			gqlErr.Extensions = map[string]interface{}{}
		}
		gqlErr.Extensions["code"] = string(errorCodeFor(e))
		return gqlErr
	})

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
	e.GET("/events", sse.NewHandler(sseBroker, sseTicketRepository))

	termsBroadcastScheduler.SchedulePending(context.Background())
	go func() {
		defer close(activityArchiveDone)
		activityArchiver.Run(activityArchiveCtx)
	}()

	go func() {
		if err := e.Start(":8080"); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info().Msg("shutting down server...")
	stopActivityArchive()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Log.Error().Err(err).Msg("server shutdown error")
	}
	select {
	case <-activityArchiveDone:
	case <-shutdownCtx.Done():
		logger.Log.Error().Err(shutdownCtx.Err()).Msg("activity archive did not stop before shutdown")
	}
	if err := courseImportTracker.Shutdown(shutdownCtx); err != nil {
		logger.Log.Error().Err(err).Msg("course import did not stop before shutdown; leaving DB and Redis open")
		return
	}

	// リクエストの外で走っている処理（チャット配信・お知らせ通知・活動記録）を、
	// DB 接続を閉じる前に片付ける。ここで待たないと、処理中の書き込みが
	// 「閉じた DB」に当たって全部失敗する。待ちきれなかったぶんは諦める
	// （どれもベストエフォート。chat.EventPublisher 参照）。
	for _, runner := range asyncRunners {
		if err := runner.Wait(shutdownCtx); err != nil {
			logger.Log.Error().Err(err).Msg("async tasks did not drain before shutdown")
			// 1つ待ちきれなければ残りも待ちきれない（同じ shutdownCtx のため）。
			break
		}
	}

	if err := database.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("database close error")
	}
	if err := redisClient.Close(); err != nil {
		logger.Log.Error().Err(err).Msg("redis close error")
	}

	logger.Log.Info().Msg("server stopped")
}

// errorCodeFor maps a resolver error to a stable machine-readable code exposed
// via GraphQL extensions.code. Typed *apperr.Error carry their own code; legacy
// auth errors (from JWT validation middleware) are matched by message so they
// also carry a code without every call site being rewritten.
func errorCodeFor(err error) apperr.Code {
	if code := apperr.CodeOf(err); code != apperr.CodeInternal {
		return code
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "invalid token"),
		strings.Contains(msg, "token has been revoked"),
		strings.Contains(msg, "token is expired"),
		strings.Contains(msg, "missing authorization"):
		return apperr.CodeUnauthorized
	case strings.Contains(msg, "forbidden"):
		return apperr.CodeForbidden
	default:
		return apperr.CodeInternal
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

// originCheckingWebsocketImplementation は gqlgen の WebSocket 実装をラップし、
// ハンドシェイク前に Origin ヘッダーを isOriginAllowed で検証する。
//
// gqlgen v0.17.95 で transport.Websocket.Upgrader (gorilla/websocket) が廃止され、
// 実装が coder/websocket に移行した。coder/websocket の既定のオリジン検証は
// 「Origin のホストがリクエストホストと一致する場合のみ許可（Origin 無しは許可）」
// であり、従来の許可リスト方式とは意味が異なる。そのため coder 側の検証は
// InsecureSkipVerify で無効化し、代わりに従来と同一の isOriginAllowed を
// Accept の前段で適用することで挙動を完全に保つ。
type originCheckingWebsocketImplementation struct {
	allowedOrigins []string
	inner          transport.WebsocketImplementation
}

func newOriginCheckingWebsocketImplementation(allowedOrigins []string) originCheckingWebsocketImplementation {
	return originCheckingWebsocketImplementation{
		allowedOrigins: allowedOrigins,
		inner: transport.CoderWebsocketImplementation{
			AcceptOptions: coderws.AcceptOptions{
				// オリジン検証は下の isOriginAllowed が担当するため、
				// coder/websocket 側の既定検証は無効化する。
				InsecureSkipVerify: true,
			},
		},
	}
}

func (i originCheckingWebsocketImplementation) Accept(
	w http.ResponseWriter,
	r *http.Request,
	options transport.WebsocketAcceptOptions,
) (transport.WebsocketConn, error) {
	if !isOriginAllowed(r.Header.Get("Origin"), i.allowedOrigins) {
		return nil, fmt.Errorf("websocket: request origin %q not allowed", r.Header.Get("Origin"))
	}
	return i.inner.Accept(w, r, options)
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
