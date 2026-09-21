// Package di は、リポジトリからユースケースを組み立てる配線。
//
// 以前は graph パッケージに置いてあった。GraphQL の層が「どのリポジトリから
// どのユースケースを作るか」を知っていることになり、依存の向きが逆立ちする
// （本来 graph は組み立て済みのものを受け取るだけでよい）。別の入口を足すときも、
// その入口が graph を import しないと配線を再利用できなくなる。
//
// 返す型が graph.XxxUseCases なのは、リゾルバがその形で束ねて受け取るため。
// di → graph の一方向で、graph は di を知らない。
package di

import (
	"github.com/Cityboypenguin/SPACE-server/graph"
	"github.com/Cityboypenguin/SPACE-server/repository"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	pollusecase "github.com/Cityboypenguin/SPACE-server/usecase/poll"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
	semesterusecase "github.com/Cityboypenguin/SPACE-server/usecase/semester"
	timetableusecase "github.com/Cityboypenguin/SPACE-server/usecase/timetable"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

// NewCommunityUseCases wires every community use case from its repositories and
// returns the grouped struct. Centralizing the wiring here keeps main.go free of
// the per-use-case construction boilerplate and means adding a community use case
// touches one place instead of three.
func NewCommunityUseCases(
	uploads uploadusecase.Acceptor,
	communityRepo repository.CommunityRepository,
	roomUserRepo repository.RoomUserRepository,
	txManager repository.TxManager,
) graph.CommunityUseCases {
	return graph.CommunityUseCases{
		CreateCommunityUseCase:        communityusecase.NewCreateCommunityUseCase(uploads, communityRepo),
		GetCommunityUseCase:           communityusecase.NewGetCommunityUseCase(communityRepo),
		UpdateCommunityUseCase:        communityusecase.NewUpdateCommunityUseCase(uploads, communityRepo, roomUserRepo),
		UpdateCommunityMembersUseCase: communityusecase.NewUpdateCommunityMembersUseCase(communityRepo, roomUserRepo, txManager),
		SearchCommunityUseCase:        communityusecase.NewSearchCommunityUseCase(communityRepo),
		ListMyCommunitiesUseCase:      communityusecase.NewListMyCommunitiesUseCase(communityRepo),
		ListAllCommunitiesUseCase:     communityusecase.NewListAllCommunitiesUseCase(communityRepo),
		GetRandomCommunitiesUseCase:   *communityusecase.NewGetRandomCommunitiesUseCase(communityRepo),
	}
}

// NewCourseUseCases wires every course/timetable/current-semester use case from its
// repositories, following the same grouping pattern as NewCommunityUseCases.
func NewCourseUseCases(
	courseRepo repository.CourseRepository,
	timetableRepo repository.TimetableRepository,
	settingRepo repository.SystemSettingRepository,
	anonIdentityRepo repository.RoomAnonymousIdentityRepository,
	userSettingRepo repository.UserSettingRepository,
	roomRepo repository.RoomRepository,
	blockRepo repository.BlockerRepository,
	// 授業一覧の未読バッジしか使わないので、合成インターフェースではなく
	// 未読集計の口だけを受け取る。
	unreadCounter repository.MessageUnreadCounter,
) graph.CourseUseCases {
	return graph.CourseUseCases{
		SearchCoursesUseCase:               courseusecase.NewSearchCoursesUseCase(courseRepo, settingRepo),
		GetCourseByIDUseCase:               courseusecase.NewGetCourseByIDUseCase(courseRepo),
		RegisterTimetableUseCase:           timetableusecase.NewRegisterTimetableUseCase(timetableRepo),
		RemoveTimetableUseCase:             timetableusecase.NewRemoveTimetableUseCase(timetableRepo),
		SetTimetableEntryColorUseCase:      timetableusecase.NewSetTimetableEntryColorUseCase(timetableRepo),
		ListTimetableUseCase:               timetableusecase.NewListTimetableUseCase(timetableRepo, settingRepo),
		ReplaceTimetableUseCase:            timetableusecase.NewReplaceTimetableUseCase(timetableRepo),
		GetUserTimetableUseCase:            timetableusecase.NewGetUserTimetableUseCase(timetableRepo, settingRepo, userSettingRepo, blockRepo),
		AdminRegisterTimetableUseCase:      timetableusecase.NewAdminRegisterTimetableUseCase(timetableRepo),
		AdminRemoveTimetableUseCase:        timetableusecase.NewAdminRemoveTimetableUseCase(timetableRepo),
		AdminSetTimetableEntryColorUseCase: timetableusecase.NewAdminSetTimetableEntryColorUseCase(timetableRepo),
		AdminReplaceTimetableUseCase:       timetableusecase.NewAdminReplaceTimetableUseCase(timetableRepo),
		GetCurrentSemesterUseCase:          semesterusecase.NewGetCurrentSemesterUseCase(settingRepo),
		UpdateCurrentSemesterUseCase:       semesterusecase.NewUpdateCurrentSemesterUseCase(settingRepo),
		ListCourseRoomUnreadCountsUseCase:  courseusecase.NewListCourseRoomUnreadCountsUseCase(unreadCounter, settingRepo),
		ImportCoursesUseCase:               courseusecase.NewImportCoursesUseCase(courseRepo),
		ListCoursesUseCase:                 courseusecase.NewListCoursesUseCase(courseRepo),
		ListCourseYearsUseCase:             courseusecase.NewListCourseYearsUseCase(courseRepo),
		ListDedupKeysByYearUseCase:         courseusecase.NewListDedupKeysByYearUseCase(courseRepo),
		AdminCreateCourseUseCase:           courseusecase.NewAdminCreateCourseUseCase(courseRepo),
		AdminDeleteCourseUseCase:           courseusecase.NewAdminDeleteCourseUseCase(courseRepo, roomRepo),
		GetCourseRegisteredCountUseCase:    courseusecase.NewGetCourseRegisteredCountUseCase(timetableRepo),
		GetCourseRegisteredCountsUseCase:   courseusecase.NewGetCourseRegisteredCountsUseCase(timetableRepo),
	}
}

// NewQuestionUseCases wires every question/answer use case (F-04-2 質問箱) from its
// repositories, following the same grouping pattern as NewCommunityUseCases.
func NewQuestionUseCases(
	uploads uploadusecase.Acceptor,
	events classroomEvents,
	questionRepo repository.QuestionRepository,
	answerRepo repository.AnswerRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
	courseRepo repository.CourseRepository,
	settingRepo repository.SystemSettingRepository,
	timetableRepo repository.TimetableRepository,
	// 匿名ID(匿名NNN)は投稿時に確定させるので、質問・回答の作成にも採番の口が要る。
	anonIdentityRepo repository.RoomAnonymousIdentityRepository,
) graph.QuestionUseCases {
	requireWritable := courseusecase.NewRequireWritableCourseRoomUseCase(courseRepo, settingRepo, timetableRepo)
	anonIdentity := anonusecase.NewGetOrCreateAnonymousIdentityUseCase(anonIdentityRepo)
	return graph.QuestionUseCases{
		CreateQuestionUseCase:   questionusecase.NewCreateQuestionUseCase(events, uploads, questionRepo, mediaRepo, txManager, requireWritable, anonIdentity),
		UpdateQuestionUseCase:   questionusecase.NewUpdateQuestionUseCase(events, questionRepo, mediaRepo, txManager, requireWritable),
		ListQuestionsUseCase:    questionusecase.NewListQuestionsUseCase(questionRepo),
		GetQuestionByIDUseCase:  questionusecase.NewGetQuestionByIDUseCase(questionRepo),
		SelectBestAnswerUseCase: questionusecase.NewSelectBestAnswerUseCase(events, questionRepo, answerRepo, requireWritable),
		CancelBestAnswerUseCase: questionusecase.NewCancelBestAnswerUseCase(events, questionRepo, requireWritable),
		DeleteQuestionUseCase:   questionusecase.NewDeleteQuestionUseCase(events, questionRepo),
		DeleteMyQuestionUseCase: questionusecase.NewDeleteMyQuestionUseCase(events, questionRepo, requireWritable),
		AnswerQuestionUseCase:   answerusecase.NewAnswerQuestionUseCase(events, uploads, questionRepo, answerRepo, mediaRepo, txManager, requireWritable, anonIdentity),
		UpdateAnswerUseCase:     answerusecase.NewUpdateAnswerUseCase(events, questionRepo, answerRepo, mediaRepo, txManager, requireWritable),
		DeleteAnswerUseCase:     answerusecase.NewDeleteAnswerUseCase(events, questionRepo, answerRepo, requireWritable),
		LikeAnswerUseCase:       answerusecase.NewLikeAnswerUseCase(events, questionRepo, answerRepo, requireWritable),
		UnlikeAnswerUseCase:     answerusecase.NewUnlikeAnswerUseCase(events, questionRepo, answerRepo, requireWritable),
	}
}

// NewPollUseCases wires every poll use case (F-04-3 投票) from its repositories,
// following the same grouping pattern as NewCommunityUseCases.
func NewPollUseCases(
	events classroomEvents,
	pollRepo repository.PollRepository,
	courseRepo repository.CourseRepository,
	settingRepo repository.SystemSettingRepository,
	timetableRepo repository.TimetableRepository,
	// 匿名ID(匿名NNN)は投稿時に確定させるので、投票の作成にも採番の口が要る。
	anonIdentityRepo repository.RoomAnonymousIdentityRepository,
) graph.PollUseCases {
	requireWritable := courseusecase.NewRequireWritableCourseRoomUseCase(courseRepo, settingRepo, timetableRepo)
	return graph.PollUseCases{
		CreatePollUseCase:  pollusecase.NewCreatePollUseCase(events, pollRepo, requireWritable, anonusecase.NewGetOrCreateAnonymousIdentityUseCase(anonIdentityRepo)),
		VotePollUseCase:    pollusecase.NewVotePollUseCase(events, pollRepo, requireWritable),
		DeletePollUseCase:  pollusecase.NewDeletePollUseCase(events, pollRepo, requireWritable),
		ListPollsUseCase:   pollusecase.NewListPollsUseCase(pollRepo),
		GetPollByIDUseCase: pollusecase.NewGetPollByIDUseCase(pollRepo),
	}
}

// classroomEvents は質問・回答・投票の配信の出口をまとめて受け取るための口。
//
// 3つのポート（usecase/question, usecase/answer, usecase/poll の EventPublisher）を
// 1つの実装が満たすので、配線もまとめて受け取る。graph.NewClassroomEventPublisher が
// これを満たす。
type classroomEvents interface {
	questionusecase.EventPublisher
	answerusecase.EventPublisher
	pollusecase.EventPublisher
}
