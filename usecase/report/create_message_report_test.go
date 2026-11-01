package report

import (
	"context"
	"errors"
	"testing"
	"time"

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

type fakeMediaReader struct {
	repository.MediaReader
	media map[int64][]*model.Media
}

func (f *fakeMediaReader) ListByMessageIDs(context.Context, []int64) (map[int64][]*model.Media, error) {
	return f.media, nil
}

type fakeUsers struct{ user *model.User }

func (f *fakeUsers) Execute(context.Context, int64) (*model.User, error) { return f.user, nil }

type messageReportFixture struct {
	repo *fakeReportRepo
	deps CreateMessageReportDeps
}

func newMessageReportFixture(roomType string) *messageReportFixture {
	repo := &fakeReportRepo{}
	created := time.Unix(1700000000, 0)
	return &messageReportFixture{
		repo: repo,
		deps: CreateMessageReportDeps{
			Reports: newUsecase(repo, true),
			Access:  &fakeRoomAccess{room: &model.Room{ID: 5, Type: roomType, Name: "room"}},
			Messages: &fakeMessageReader{msg: &model.Message{
				ID: 9, RoomID: 5, UserID: 2, Content: "通報時点の本文", CreatedAt: created, UpdatedAt: created,
			}},
			Media: &fakeMediaReader{media: map[int64][]*model.Media{
				9: {{ID: 3, StorageKey: "media/a.png", ContentType: "image/png", CreatedAt: created}},
			}},
			Users: &fakeUsers{user: &model.User{ID: 2, Name: "実名", AccountID: "real_account"}},
		},
	}
}

func (f *messageReportFixture) execute() (*model.Report, error) {
	return NewCreateMessageReportUsecase(f.deps).Execute(context.Background(), CreateMessageReportInput{
		ReporterID: 1,
		MessageID:  9,
		Reason:     "SPAM",
	})
}

func TestCreateMessageReport_SavesSnapshot(t *testing.T) {
	f := newMessageReportFixture(model.RoomTypeCommunity)
	if _, err := f.execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	saved := f.repo.saved
	if saved == nil || saved.TargetType != model.TargetMessage || saved.TargetID != "9" {
		t.Fatalf("saved = %+v, want a MESSAGE report for 9", saved)
	}
	if saved.PostContent != nil {
		t.Fatalf("PostContent = %q; the plaintext content column must stay empty", *saved.PostContent)
	}
	s := saved.ReportedMessage
	if s == nil {
		t.Fatal("ReportedMessage is nil")
	}
	if s.Content != "通報時点の本文" || s.AuthorName != "実名" || s.AuthorAccountID != "real_account" {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.AuthorID == nil || *s.AuthorID != 2 {
		t.Fatalf("AuthorID = %v, want 2", s.AuthorID)
	}
	if len(s.Media) != 1 || s.Media[0].StorageKey != "media/a.png" {
		t.Fatalf("Media = %+v", s.Media)
	}
}

func TestCreateMessageReport_RejectsWithoutReadAccess(t *testing.T) {
	f := newMessageReportFixture(model.RoomTypeDM)
	denied := errors.New("forbidden: not a member of this room")
	f.deps.Access = &fakeRoomAccess{err: denied}
	if _, err := f.execute(); !errors.Is(err, denied) {
		t.Fatalf("err = %v, want %v", err, denied)
	}
	if f.repo.saved != nil {
		t.Fatal("must not copy a message the reporter cannot read")
	}
}

func TestCreateMessageReport_RejectsMissingAndOwnMessages(t *testing.T) {
	f := newMessageReportFixture(model.RoomTypeCommunity)
	f.deps.Messages = &fakeMessageReader{}
	if _, err := f.execute(); err == nil {
		t.Fatal("expected an error for a missing or deleted message")
	}

	f = newMessageReportFixture(model.RoomTypeCommunity)
	f.deps.Messages = &fakeMessageReader{msg: &model.Message{ID: 9, RoomID: 5, UserID: 1}}
	if _, err := f.execute(); err == nil {
		t.Fatal("expected an error when reporting one's own message")
	}
	if f.repo.saved != nil {
		t.Fatal("rejected reports must not be saved")
	}
}

func TestCreateMessageReport_DeletedAuthorKeepsID(t *testing.T) {
	f := newMessageReportFixture(model.RoomTypeDM)
	f.deps.Users = &fakeUsers{}
	if _, err := f.execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	s := f.repo.saved.ReportedMessage
	if s.AuthorID == nil || *s.AuthorID != 2 || s.AuthorName != "" {
		t.Fatalf("snapshot = %+v", s)
	}
}
