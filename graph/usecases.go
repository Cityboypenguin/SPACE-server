package graph

import (
	"github.com/Cityboypenguin/SPACE-server/repository"
	anonusecase "github.com/Cityboypenguin/SPACE-server/usecase/anon"
	answerusecase "github.com/Cityboypenguin/SPACE-server/usecase/answer"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
	pollusecase "github.com/Cityboypenguin/SPACE-server/usecase/poll"
	questionusecase "github.com/Cityboypenguin/SPACE-server/usecase/question"
	semesterusecase "github.com/Cityboypenguin/SPACE-server/usecase/semester"
	timetableusecase "github.com/Cityboypenguin/SPACE-server/usecase/timetable"
)

// NewCommunityUseCases wires every community use case from its repositories and
// returns the grouped struct. Centralizing the wiring here keeps main.go free of
// the per-use-case construction boilerplate and means adding a community use case
// touches one place instead of three.
func NewCommunityUseCases(
	communityRepo repository.CommunityRepository,
	mediaRepo repository.MediaRepository,
	roomUserRepo repository.RoomUserRepository,
	txManager repository.TxManager,
) CommunityUseCases {
	return CommunityUseCases{
		CreateCommunityUseCase:             communityusecase.NewCreateCommunityUseCase(communityRepo, mediaRepo),
		GetCommunityUseCase:                communityusecase.NewGetCommunityUseCase(communityRepo),
		UpdateCommunityUseCase:             communityusecase.NewUpdateCommunityUseCase(communityRepo),
		UpdateCommunityMembersUseCase:      communityusecase.NewUpdateCommunityMembersUseCase(communityRepo, roomUserRepo, txManager),
		SearchCommunityUseCase:             communityusecase.NewSearchCommunityUseCase(communityRepo),
		ListMyCommunitiesUseCase:           communityusecase.NewListMyCommunitiesUseCase(communityRepo),
		ListAllCommunitiesUseCase:          communityusecase.NewListAllCommunitiesUseCase(communityRepo),
		PromoteToCommunityOwnerUseCase:     communityusecase.NewPromoteToCommunityOwnerUseCase(communityRepo, roomUserRepo),
		DemoteFromCommunityOwnerUseCase:    communityusecase.NewDemoteFromCommunityOwnerUseCase(communityRepo, roomUserRepo),
		IsSoleOwnerWithOtherMembersUseCase: communityusecase.NewIsSoleOwnerWithOtherMembersUseCase(communityRepo),
		GetRandomCommunitiesUseCase:        *communityusecase.NewGetRandomCommunitiesUseCase(communityRepo),
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
) CourseUseCases {
	return CourseUseCases{
		SearchCoursesUseCase:                courseusecase.NewSearchCoursesUseCase(courseRepo, settingRepo),
		GetCourseByIDUseCase:                courseusecase.NewGetCourseByIDUseCase(courseRepo),
		RegisterTimetableUseCase:            timetableusecase.NewRegisterTimetableUseCase(timetableRepo),
		RemoveTimetableUseCase:              timetableusecase.NewRemoveTimetableUseCase(timetableRepo),
		SetTimetableEntryColorUseCase:       timetableusecase.NewSetTimetableEntryColorUseCase(timetableRepo),
		ListTimetableUseCase:                timetableusecase.NewListTimetableUseCase(timetableRepo, settingRepo),
		ReplaceTimetableUseCase:             timetableusecase.NewReplaceTimetableUseCase(timetableRepo),
		GetUserTimetableUseCase:             timetableusecase.NewGetUserTimetableUseCase(timetableRepo, settingRepo, userSettingRepo),
		AdminRegisterTimetableUseCase:       timetableusecase.NewAdminRegisterTimetableUseCase(timetableRepo),
		AdminRemoveTimetableUseCase:         timetableusecase.NewAdminRemoveTimetableUseCase(timetableRepo),
		AdminSetTimetableEntryColorUseCase:  timetableusecase.NewAdminSetTimetableEntryColorUseCase(timetableRepo),
		AdminReplaceTimetableUseCase:        timetableusecase.NewAdminReplaceTimetableUseCase(timetableRepo),
		GetCurrentSemesterUseCase:           semesterusecase.NewGetCurrentSemesterUseCase(settingRepo),
		UpdateCurrentSemesterUseCase:        semesterusecase.NewUpdateCurrentSemesterUseCase(settingRepo),
		CheckRoomWritableUseCase:            courseusecase.NewCheckRoomWritableUseCase(courseRepo, settingRepo, timetableRepo),
		GetOrCreateAnonymousIdentityUseCase: anonusecase.NewGetOrCreateAnonymousIdentityUseCase(anonIdentityRepo),
		ImportCoursesUseCase:                courseusecase.NewImportCoursesUseCase(courseRepo),
		ListCoursesUseCase:                  courseusecase.NewListCoursesUseCase(courseRepo),
		ListCourseYearsUseCase:              courseusecase.NewListCourseYearsUseCase(courseRepo),
		ListDedupKeysByYearUseCase:          courseusecase.NewListDedupKeysByYearUseCase(courseRepo),
	}
}

// NewQuestionUseCases wires every question/answer use case (F-04-2 質問箱) from its
// repositories, following the same grouping pattern as NewCommunityUseCases.
func NewQuestionUseCases(
	questionRepo repository.QuestionRepository,
	answerRepo repository.AnswerRepository,
	mediaRepo repository.MediaRepository,
	txManager repository.TxManager,
	courseRepo repository.CourseRepository,
	settingRepo repository.SystemSettingRepository,
	timetableRepo repository.TimetableRepository,
) QuestionUseCases {
	requireWritable := courseusecase.NewRequireWritableCourseRoomUseCase(courseRepo, settingRepo, timetableRepo)
	return QuestionUseCases{
		CreateQuestionUseCase:   questionusecase.NewCreateQuestionUseCase(questionRepo, mediaRepo, txManager, requireWritable),
		UpdateQuestionUseCase:   questionusecase.NewUpdateQuestionUseCase(questionRepo),
		ListQuestionsUseCase:    questionusecase.NewListQuestionsUseCase(questionRepo),
		GetQuestionByIDUseCase:  questionusecase.NewGetQuestionByIDUseCase(questionRepo),
		SelectBestAnswerUseCase: questionusecase.NewSelectBestAnswerUseCase(questionRepo, answerRepo),
		CancelBestAnswerUseCase: questionusecase.NewCancelBestAnswerUseCase(questionRepo),
		DeleteQuestionUseCase:   questionusecase.NewDeleteQuestionUseCase(questionRepo),
		DeleteMyQuestionUseCase: questionusecase.NewDeleteMyQuestionUseCase(questionRepo),
		AnswerQuestionUseCase:   answerusecase.NewAnswerQuestionUseCase(questionRepo, answerRepo, mediaRepo, txManager, requireWritable),
		ListAnswersUseCase:      answerusecase.NewListAnswersUseCase(answerRepo),
		GetAnswerByIDUseCase:    answerusecase.NewGetAnswerByIDUseCase(answerRepo),
		UpdateAnswerUseCase:     answerusecase.NewUpdateAnswerUseCase(questionRepo, answerRepo),
		DeleteAnswerUseCase:     answerusecase.NewDeleteAnswerUseCase(questionRepo, answerRepo),
		LikeAnswerUseCase:       answerusecase.NewLikeAnswerUseCase(answerRepo),
		UnlikeAnswerUseCase:     answerusecase.NewUnlikeAnswerUseCase(answerRepo),
	}
}

// NewPollUseCases wires every poll use case (F-04-3 投票) from its repositories,
// following the same grouping pattern as NewCommunityUseCases.
func NewPollUseCases(
	pollRepo repository.PollRepository,
	courseRepo repository.CourseRepository,
	settingRepo repository.SystemSettingRepository,
	timetableRepo repository.TimetableRepository,
) PollUseCases {
	requireWritable := courseusecase.NewRequireWritableCourseRoomUseCase(courseRepo, settingRepo, timetableRepo)
	return PollUseCases{
		CreatePollUseCase:            pollusecase.NewCreatePollUseCase(pollRepo, requireWritable),
		VotePollUseCase:              pollusecase.NewVotePollUseCase(pollRepo, requireWritable),
		DeletePollUseCase:            pollusecase.NewDeletePollUseCase(pollRepo),
		ListPollsUseCase:             pollusecase.NewListPollsUseCase(pollRepo),
		GetPollByIDUseCase:           pollusecase.NewGetPollByIDUseCase(pollRepo),
		ListPollOptionResultsUseCase: pollusecase.NewListPollOptionResultsUseCase(pollRepo),
	}
}
