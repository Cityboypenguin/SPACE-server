package report

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeRoomAccess struct {
	room *model.Room
	err  error
}

func (f *fakeRoomAccess) EnsureReadAccess(context.Context, int64) (*model.Room, error) {
	return f.room, f.err
}

type fakeMessageReader struct {
	repository.MessageReader
	msg *model.Message
}

func (f *fakeMessageReader) GetMessageByID(context.Context, int64) (*model.Message, error) {
	return f.msg, nil
}

type fakeQuestionReader struct{ question *model.Question }

func (f *fakeQuestionReader) GetQuestionByID(context.Context, int64) (*model.Question, error) {
	return f.question, nil
}

type fakeAnswerReader struct{ answer *model.Answer }

func (f *fakeAnswerReader) GetAnswerByID(context.Context, int64) (*model.Answer, error) {
	return f.answer, nil
}

type fakePollReader struct {
	poll    *model.Poll
	options []*repository.PollOptionResult
}

func (f *fakePollReader) GetPollByID(context.Context, int64) (*model.Poll, error) {
	return f.poll, nil
}

func (f *fakePollReader) ListOptionsWithResults(context.Context, int64, int64) ([]*repository.PollOptionResult, error) {
	return f.options, nil
}

type fakeMediaReader struct {
	repository.MediaReader
	messages  map[int64][]*model.Media
	questions map[int64][]*model.Media
	answers   map[int64][]*model.Media
}

type fakeAddendumReader struct{}

func (fakeAddendumReader) ListQuestionAddenda(_ context.Context, ids []int64) (map[int64][]*model.Addendum, error) {
	return map[int64][]*model.Addendum{ids[0]: {{Body: "質問への追記"}}}, nil
}

func (fakeAddendumReader) ListAnswerAddenda(_ context.Context, ids []int64) (map[int64][]*model.Addendum, error) {
	return map[int64][]*model.Addendum{ids[0]: {{Body: "回答への追記"}}}, nil
}

func (f *fakeMediaReader) ListByMessageIDs(context.Context, []int64) (map[int64][]*model.Media, error) {
	return f.messages, nil
}

func (f *fakeMediaReader) ListByQuestionIDs(context.Context, []int64) (map[int64][]*model.Media, error) {
	return f.questions, nil
}

func (f *fakeMediaReader) ListByAnswerIDs(context.Context, []int64) (map[int64][]*model.Media, error) {
	return f.answers, nil
}

type fakeUsers struct{ user *model.User }

func (f *fakeUsers) Execute(context.Context, int64) (*model.User, error) { return f.user, nil }

type contentReportFixture struct {
	repo *fakeReportRepo
	deps CreateContentReportDeps
}

func newContentReportFixture(roomType string) *contentReportFixture {
	repo := &fakeReportRepo{}
	created := time.Unix(1700000000, 0)
	media := func(key string) map[int64][]*model.Media {
		return map[int64][]*model.Media{9: {{ID: 3, StorageKey: key, ContentType: "image/png", CreatedAt: created}}}
	}
	return &contentReportFixture{
		repo: repo,
		deps: CreateContentReportDeps{
			Reports: newUsecase(repo, true),
			Access:  &fakeRoomAccess{room: &model.Room{ID: 5, Type: roomType, Name: "room"}},
			Messages: &fakeMessageReader{msg: &model.Message{
				ID: 9, RoomID: 5, UserID: 2, Content: "通報時点の本文", CreatedAt: created, UpdatedAt: created,
			}},
			Questions: &fakeQuestionReader{question: &model.Question{
				ID: 9, RoomID: 5, AskerUserID: 2, Body: "質問の本文", CreatedAt: created, UpdatedAt: created,
			}},
			Answers: &fakeAnswerReader{answer: &model.Answer{
				ID: 9, QuestionID: 7, AuthorUserID: 2, Body: "回答の本文", CreatedAt: created, UpdatedAt: created,
			}},
			Polls: &fakePollReader{
				poll: &model.Poll{ID: 9, RoomID: 5, AuthorUserID: 2, Question: "投票の設問", CreatedAt: created, UpdatedAt: created},
				options: []*repository.PollOptionResult{
					{Option: &model.PollOption{Label: "選択肢A"}, VoteCount: 3, VotedByMe: true},
					{Option: &model.PollOption{Label: "選択肢B"}},
				},
			},
			Addenda: fakeAddendumReader{},
			Media: &fakeMediaReader{
				messages:  media("media/message.png"),
				questions: media("media/question.png"),
				answers:   media("media/answer.png"),
			},
			Users: &fakeUsers{user: &model.User{ID: 2, Name: "実名", AccountID: "real_account"}},
		},
	}
}

func (f *contentReportFixture) execute(target model.ReportTargetType) (*model.Report, error) {
	return NewCreateContentReportUsecase(f.deps).Execute(context.Background(), CreateContentReportInput{
		ReporterID: 1,
		TargetType: target,
		TargetID:   9,
		Reason:     "SPAM",
	})
}

func TestCreateContentReport_CopiesAddenda(t *testing.T) {
	for target, want := range map[model.ReportTargetType]string{
		model.TargetQuestion: "質問への追記",
		model.TargetAnswer:   "回答への追記",
	} {
		f := newContentReportFixture(model.RoomTypeCourse)
		if _, err := f.execute(target); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if got := f.repo.saved.ReportedContent.Addenda; len(got) != 1 || got[0] != want {
			t.Errorf("%s: addenda = %v, want [%s]", target, got, want)
		}
	}
}

func TestCreateContentReport_CopiesTheStampOfAStampMessage(t *testing.T) {
	stamp := "stamp:ryokai"
	f := newContentReportFixture(model.RoomTypeCourse)
	f.deps.Messages = &fakeMessageReader{msg: &model.Message{ID: 9, RoomID: 5, UserID: 2, StampID: &stamp}}
	if _, err := f.execute(model.TargetMessage); err != nil {
		t.Fatal(err)
	}
	if got := f.repo.saved.ReportedContent.StampID; got == nil || *got != stamp {
		t.Errorf("stamp = %v, want %s", got, stamp)
	}
}

func TestCreateContentReport_SavesSnapshot(t *testing.T) {
	question := "質問の本文"
	cases := []struct {
		target      model.ReportTargetType
		content     string
		mediaKey    string
		parentBody  *string
		pollOptions []string
	}{
		{target: model.TargetMessage, content: "通報時点の本文", mediaKey: "media/message.png"},
		{target: model.TargetQuestion, content: "質問の本文", mediaKey: "media/question.png"},
		{target: model.TargetAnswer, content: "回答の本文", mediaKey: "media/answer.png", parentBody: &question},
		{target: model.TargetPoll, content: "投票の設問", pollOptions: []string{"選択肢A", "選択肢B"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.target), func(t *testing.T) {
			f := newContentReportFixture(model.RoomTypeCourse)
			if _, err := f.execute(tc.target); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			saved := f.repo.saved
			if saved == nil || saved.TargetType != tc.target || saved.TargetID != "9" {
				t.Fatalf("saved = %+v", saved)
			}
			if saved.PostContent != nil {
				t.Fatalf("PostContent = %q; the plaintext content column must stay empty", *saved.PostContent)
			}
			s := saved.ReportedContent
			if s == nil || s.Content != tc.content || s.ContentID != 9 || s.RoomID != 5 {
				t.Fatalf("snapshot = %+v", s)
			}
			if s.AuthorID == nil || *s.AuthorID != 2 || s.AuthorName != "実名" || s.AuthorAccountID != "real_account" {
				t.Fatalf("author = %+v", s)
			}
			if tc.mediaKey == "" && len(s.Media) != 0 {
				t.Fatalf("Media = %+v", s.Media)
			}
			if tc.mediaKey != "" && (len(s.Media) != 1 || s.Media[0].StorageKey != tc.mediaKey) {
				t.Fatalf("Media = %+v, want %s", s.Media, tc.mediaKey)
			}
			if (tc.parentBody == nil) != (s.ParentBody == nil) || (tc.parentBody != nil && *s.ParentBody != *tc.parentBody) {
				t.Fatalf("ParentBody = %v", s.ParentBody)
			}
			if !reflect.DeepEqual(s.PollOptions, tc.pollOptions) {
				t.Fatalf("PollOptions = %v, want %v", s.PollOptions, tc.pollOptions)
			}
		})
	}
}

func TestCreateContentReport_RejectsWithoutReadAccess(t *testing.T) {
	for _, target := range []model.ReportTargetType{model.TargetMessage, model.TargetQuestion, model.TargetAnswer, model.TargetPoll} {
		f := newContentReportFixture(model.RoomTypeDM)
		denied := errors.New("forbidden: not a member of this room")
		f.deps.Access = &fakeRoomAccess{err: denied}
		if _, err := f.execute(target); !errors.Is(err, denied) {
			t.Fatalf("%s: err = %v, want %v", target, err, denied)
		}
		if f.repo.saved != nil {
			t.Fatalf("%s: must not copy content the reporter cannot read", target)
		}
	}
}

func TestCreateContentReport_RejectsMissingTargets(t *testing.T) {
	cases := map[model.ReportTargetType]func(*CreateContentReportDeps){
		model.TargetMessage:  func(d *CreateContentReportDeps) { d.Messages = &fakeMessageReader{} },
		model.TargetQuestion: func(d *CreateContentReportDeps) { d.Questions = &fakeQuestionReader{} },
		model.TargetAnswer:   func(d *CreateContentReportDeps) { d.Answers = &fakeAnswerReader{} },
		model.TargetPoll:     func(d *CreateContentReportDeps) { d.Polls = &fakePollReader{} },
	}
	for target, empty := range cases {
		f := newContentReportFixture(model.RoomTypeCourse)
		empty(&f.deps)
		if _, err := f.execute(target); apperr.CodeOf(err) != apperr.CodeNotFound {
			t.Fatalf("%s: err = %v, want not found", target, err)
		}
	}

	// 質問が消えた回答はルームが分からないので通報できない。
	f := newContentReportFixture(model.RoomTypeCourse)
	f.deps.Questions = &fakeQuestionReader{}
	if _, err := f.execute(model.TargetAnswer); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("answer without question: err = %v, want not found", err)
	}
}

func TestCreateContentReport_RejectsOwnContent(t *testing.T) {
	cases := map[model.ReportTargetType]func(*CreateContentReportDeps){
		model.TargetMessage: func(d *CreateContentReportDeps) {
			d.Messages = &fakeMessageReader{msg: &model.Message{ID: 9, RoomID: 5, UserID: 1}}
		},
		model.TargetQuestion: func(d *CreateContentReportDeps) {
			d.Questions = &fakeQuestionReader{question: &model.Question{ID: 9, RoomID: 5, AskerUserID: 1}}
		},
		model.TargetAnswer: func(d *CreateContentReportDeps) {
			d.Answers = &fakeAnswerReader{answer: &model.Answer{ID: 9, QuestionID: 7, AuthorUserID: 1}}
		},
		model.TargetPoll: func(d *CreateContentReportDeps) {
			d.Polls = &fakePollReader{poll: &model.Poll{ID: 9, RoomID: 5, AuthorUserID: 1}}
		},
	}
	for target, own := range cases {
		f := newContentReportFixture(model.RoomTypeCourse)
		own(&f.deps)
		if _, err := f.execute(target); apperr.CodeOf(err) != apperr.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid input", target, err)
		}
		if f.repo.saved != nil {
			t.Fatalf("%s: rejected reports must not be saved", target)
		}
	}
}

func TestCreateContentReport_DeletedAuthorKeepsID(t *testing.T) {
	f := newContentReportFixture(model.RoomTypeDM)
	f.deps.Users = &fakeUsers{}
	if _, err := f.execute(model.TargetMessage); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	s := f.repo.saved.ReportedContent
	if s.AuthorID == nil || *s.AuthorID != 2 || s.AuthorName != "" {
		t.Fatalf("snapshot = %+v", s)
	}
}
