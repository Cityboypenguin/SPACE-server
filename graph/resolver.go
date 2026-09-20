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
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	"github.com/Cityboypenguin/SPACE-server/usecase/block"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
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
	usersettingsusecase "github.com/Cityboypenguin/SPACE-server/usecase/user_settings"
)

// CourseImportStatusTopic is the PubSub topic the course-import background job
// (wired up in cmd/server/main.go) publishes to on every state/progress change,
// letting AdminCourseImportStatusUpdated push updates to admins watching the
// import screen instead of them having to poll adminCourseImportStatus.
const CourseImportStatusTopic = "course_import:status"

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.
//
// ■ フィールドが多いことについて
//
// ここは composition root（依存を1箇所に集める場所）なので、スキーマに口が
// 増えれば依存も増える。数そのものは問題ではない。分割して困るのは、
// 「どのリゾルバがどれを使うか」の対応が別ファイルに散って、口を足すときに
// 見る場所が増えることのほう。
//
// グループ（*UseCases）に切り出してあるのは、数を減らすためではなく
// 「まとめること自体に意味がある」ものだけ:
//
//   - ChatUseCases: 権限判定を飛ばして保存処理を叩けないよう、入口を絞るため
//     （MessageRoomUseCases のコメント参照）
//   - CourseUseCases / QuestionUseCases のように、複数パッケージのユースケースが
//     1つの画面のために組で使われるもの
//
// 逆に、1つのパッケージのユースケースが並んでいるだけの塊（administrator、
// favorite、block、terms など）は空行で区切ってあるだけにしてある。構造体に
// しても増えるのは名前だけで、探しにくくなるぶん損になる。
type Resolver struct {
	StorageRepository     repository.StorageRepository
	MaintenanceRepository repository.MaintenanceRepository
	MaintenanceFlag       *atomic.Bool

	UserUseCases
	PostUseCases

	UpdateProfileUseCase   profile.UpdateProfileUseCase
	UpdateMyProfileUseCase profile.UpdateMyProfileUseCase
	GetProfileUseCase      profile.GetProfileUseCase
	SetAvatarUseCase       profile.SetAvatarUseCase
	DeleteAvatarUseCase    profile.DeleteAvatarUseCase

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

	ListMediaByPostIDUseCase           mediausecase.ListMediaByPostIDUseCase
	ReportMediaDimensionsUseCase       mediausecase.ReportDimensionsUseCase
	ListImagesMissingDimensionsUseCase mediausecase.ListImagesMissingDimensionsUseCase

	ChatUseCases

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

	// PubSub は subscription の配信口。プロセス内の実装と Redis 経由の実装を
	// 差し替えられるよう、具体型ではなく pubsub.Bus で受ける（台を増やすと、
	// 送った台に繋がっていない購読者へ届かなくなるため）。
	PubSub pubsub.Bus

	// CourseImportStatusTracker publishes to CourseImportStatusTopic via PubSub on
	// every state/progress change (see cmd/server/main.go), which
	// AdminCourseImportStatusUpdated subscribes to.
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
	// TermsBroadcastScheduler は「effectiveDate に terms_updated を配信する」タイマーの
	// 唯一の入口。起動時（cmd/server/main.go）も同じインスタンスを使う。
	TermsBroadcastScheduler *termsusecase.BroadcastScheduler

	ManageSystemSettingUsecase systemsettingsusecase.ManageSystemSettingUsecase
	ManageUserSettingUsecase   usersettingsusecase.ManageUserSettingUsecase

	GetAnalyticsUseCase          analyticsusecase.GetAnalyticsUseCase
	GetCommunityAnalyticsUseCase analyticsusecase.GetCommunityAnalyticsUseCase
	GetTimeSeriesUseCase         analyticsusecase.GetTimeSeriesUseCase
	RecordSessionUseCase         sessionusecase.RecordSessionUseCase

	// アナリティクスサマリーキャッシュの即時無効化コールバック。
	// 通報ステータス変更など管理操作が DB を更新した直後に呼ぶ。
	InvalidateAnalyticsSummary func()
}

type UserUseCases struct {
	GetUserByIDUseCase   user.GetUserByIDUseCase
	GetUsersByIDsUseCase user.GetUsersByIDsUseCase
	// 連絡先まで返す取得。本人（me）と管理者（getUserByID）だけが辿れる口で使う。
	// 表示のために引くだけなら GetUserByIDUseCase。
	GetUserAccountByIDUseCase   user.GetUserAccountByIDUseCase
	GetUserAccountsByIDsUseCase user.GetUserAccountsByIDsUseCase
	CreateUserUseCase           user.CreateUserUseCase
	SendEmailOTPUseCase         user.SendEmailOTPUseCase
	VerifyEmailOTPUseCase       user.VerifyEmailOTPUseCase
	ListUsersUseCase            user.ListUsersUseCase
	DeleteUserUseCase           user.DeleteUserUseCase
	UpdateUserUseCase           user.UpdateUserUseCase
	SearchUsersUseCase          user.SearchUsersUseCase
	// 管理画面のユーザー検索（連絡先を含む）。一般ユーザー向けは SearchUsersUseCase。
	SearchUserAccountsUseCase     user.SearchUserAccountsUseCase
	LoginUserUseCase              user.LoginUserUseCase
	RefreshUserTokenUseCase       user.RefreshUserTokenUseCase
	LogoutUserUseCase             user.LogoutUserUseCase
	FreezeUserUseCase             user.FreezeUserUseCase
	UnfreezeUserUseCase           user.UnfreezeUserUseCase
	RequestPasswordResetUseCase   user.RequestPasswordResetUseCase
	VerifyPasswordResetOTPUseCase user.VerifyPasswordResetOTPUseCase
	ResetPasswordUseCase          user.ResetPasswordUseCase
	SuggestUsersUseCase           user.SuggestUsersUseCase
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

// MessageRoomUseCases はルーム/メッセージ系のうち、リゾルバが直接使ってよいもの。
//
// メッセージの送信・編集・削除・一覧、メンション解決、既読の記録は
// ChatUseCases 経由でしか呼べないよう、ここには置いていない。
// 置いてしまうと membership・学期/履修・ブロックの判定を飛ばして
// 保存処理を直接叩ける口がリゾルバに復活してしまうため。
// なお保存処理そのものは usecase/chat/internal/messagestore に移したので、
// ここへ書き戻そうとしてもコンパイルが通らない。
type MessageRoomUseCases struct {
	GetMessageByIDUseCase           messageusecase.GetMessageByIDUseCase
	GetLastMessagesByRoomIDsUseCase messageusecase.GetLastMessagesByRoomIDsUseCase
	GetRoomUseCase                  roomusecase.GetRoomUseCase
	GetUserIDsByRoomIDUseCase       roomusecase.GetUserIDsByRoomIDUseCase
	ListUsersByRoomIDsUseCase       roomusecase.ListUsersByRoomIDsUseCase
	SearchRoomUsersUseCase          roomusecase.SearchRoomUsersUseCase
	// コミュニティ一覧の memberCount / isMember を一覧ぶん1クエリで出す口
	// （以前は1件ごとに GetUserIDsByRoomID を呼んでいた）。
	CountUsersByRoomIDsUseCase          roomusecase.CountUsersByRoomIDsUseCase
	ListJoinedRoomIDsUseCase            roomusecase.ListJoinedRoomIDsUseCase
	ListMyDMRoomsUseCase                roomusecase.ListMyDMRoomsUseCase
	GetOrCreateDMRoomUseCase            roomusecase.GetOrCreateDMRoomUseCase
	LeaveCommunityUseCase               roomusecase.LeaveCommunityUseCase
	DeleteOrphanedDMUseCase             roomusecase.DeleteOrphanedDMUseCase
	JoinRoomUseCase                     roomusecase.JoinRoomUseCase
	GetRoomUserRoleUseCase              roomusecase.GetRoomUserRoleUseCase
	SetRoomUserRoleUseCase              roomusecase.SetRoomUserRoleUseCase
	ListRoomMembersWithRolesUseCase     roomusecase.ListRoomMembersWithRolesUseCase
	ListRoomMembersWithRolesPageUseCase roomusecase.ListRoomMembersWithRolesPageUseCase
	GetRoomReadStatusBatchUseCase       roomusecase.GetRoomReadStatusBatchUseCase
	CountUnreadByRoomTypeUseCase        roomusecase.CountUnreadByRoomTypeUseCase
}

// ChatUseCases はチャット（授業内チャット・コミュニティ・DM）の業務ルールの入口。
//
// 権限判定・メンション解決・匿名IDの採番・保存・通知は全て usecase/chat 側にあり、
// リゾルバは GraphQL ID のデコードと GraphQL 型への変換だけを行う。
//
// 全部入りのサービス1つではなく責務ごとに4つ持つ。1つにまとめると、そのサービスが
// 再び20個近い依存を抱える形へ戻ってしまうため（usecase/chat のパッケージコメント
// 参照）。埋め込みにしてあるので呼び出し側は r.ChatCommands.SendMessage(...) の
// ように書ける。
type ChatUseCases struct {
	// ChatAccess は閲覧・書き込みの権限判定。他の3つもこれを共有しているので、
	// 「リゾルバで判定して、サービスでも判定して」が二重にならない。
	ChatAccess   chatusecase.AccessPolicy
	ChatCommands chatusecase.MessageCommandService
	ChatQueries  chatusecase.MessageQueryService
	ChatReads    chatusecase.ReadReceiptService
}

type CommunityUseCases struct {
	CreateCommunityUseCase        communityusecase.CreateCommunityUseCase
	GetCommunityUseCase           communityusecase.GetCommunityUseCase
	UpdateCommunityUseCase        communityusecase.UpdateCommunityUseCase
	UpdateCommunityMembersUseCase communityusecase.UpdateCommunityMembersUseCase
	SearchCommunityUseCase        communityusecase.SearchCommunityUseCase
	ListMyCommunitiesUseCase      communityusecase.ListMyCommunitiesUseCase
	ListAllCommunitiesUseCase     communityusecase.ListAllCommunitiesUseCase
	GetRandomCommunitiesUseCase   communityusecase.GetRandomCommunitiesUseCase
}

type CourseUseCases struct {
	SearchCoursesUseCase               courseusecase.SearchCoursesUseCase
	GetCourseByIDUseCase               courseusecase.GetCourseByIDUseCase
	RegisterTimetableUseCase           timetableusecase.RegisterTimetableUseCase
	RemoveTimetableUseCase             timetableusecase.RemoveTimetableUseCase
	SetTimetableEntryColorUseCase      timetableusecase.SetTimetableEntryColorUseCase
	ListTimetableUseCase               timetableusecase.ListTimetableUseCase
	ReplaceTimetableUseCase            timetableusecase.ReplaceTimetableUseCase
	GetUserTimetableUseCase            timetableusecase.GetUserTimetableUseCase
	AdminRegisterTimetableUseCase      timetableusecase.AdminRegisterTimetableUseCase
	AdminRemoveTimetableUseCase        timetableusecase.AdminRemoveTimetableUseCase
	AdminSetTimetableEntryColorUseCase timetableusecase.AdminSetTimetableEntryColorUseCase
	AdminReplaceTimetableUseCase       timetableusecase.AdminReplaceTimetableUseCase
	GetCurrentSemesterUseCase          semesterusecase.GetCurrentSemesterUseCase
	UpdateCurrentSemesterUseCase       semesterusecase.UpdateCurrentSemesterUseCase
	ListCourseRoomUnreadCountsUseCase  courseusecase.ListCourseRoomUnreadCountsUseCase
	ImportCoursesUseCase               courseusecase.ImportCoursesUseCase
	ListCoursesUseCase                 courseusecase.ListCoursesUseCase
	ListCourseYearsUseCase             courseusecase.ListCourseYearsUseCase
	ListDedupKeysByYearUseCase         courseusecase.ListDedupKeysByYearUseCase
	AdminCreateCourseUseCase           courseusecase.AdminCreateCourseUseCase
	AdminDeleteCourseUseCase           courseusecase.AdminDeleteCourseUseCase
	GetCourseRegisteredCountUseCase    courseusecase.GetCourseRegisteredCountUseCase
	GetCourseRegisteredCountsUseCase   courseusecase.GetCourseRegisteredCountsUseCase
}

type QuestionUseCases struct {
	CreateQuestionUseCase   questionusecase.CreateQuestionUseCase
	UpdateQuestionUseCase   questionusecase.UpdateQuestionUseCase
	ListQuestionsUseCase    questionusecase.ListQuestionsUseCase
	GetQuestionByIDUseCase  questionusecase.GetQuestionByIDUseCase
	SelectBestAnswerUseCase questionusecase.SelectBestAnswerUseCase
	CancelBestAnswerUseCase questionusecase.CancelBestAnswerUseCase
	DeleteQuestionUseCase   questionusecase.DeleteQuestionUseCase
	DeleteMyQuestionUseCase questionusecase.DeleteMyQuestionUseCase
	AnswerQuestionUseCase   answerusecase.AnswerQuestionUseCase
	UpdateAnswerUseCase     answerusecase.UpdateAnswerUseCase
	DeleteAnswerUseCase     answerusecase.DeleteAnswerUseCase
	LikeAnswerUseCase       answerusecase.LikeAnswerUseCase
	UnlikeAnswerUseCase     answerusecase.UnlikeAnswerUseCase
}

type PollUseCases struct {
	CreatePollUseCase  pollusecase.CreatePollUseCase
	VotePollUseCase    pollusecase.VotePollUseCase
	DeletePollUseCase  pollusecase.DeletePollUseCase
	ListPollsUseCase   pollusecase.ListPollsUseCase
	GetPollByIDUseCase pollusecase.GetPollByIDUseCase
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
	// SSE(/events) 接続用の使い捨てチケットを発行する。通知の配信経路に属するので
	// ここに置いている（internal/sse.NewHandler のコメント参照）。
	IssueStreamTicketUseCase notificationuc.IssueStreamTicketUseCase
}
