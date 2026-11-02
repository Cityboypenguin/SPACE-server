package courseimage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	chatusecase "github.com/Cityboypenguin/SPACE-server/usecase/chat"
)

type fakeImages struct {
	repository.CourseChatImageRepository
	img     *model.CourseChatImage
	deleted []int64
	order   *[]string
}

func (f *fakeImages) GetCourseChatImage(context.Context, int64) (*model.CourseChatImage, error) {
	return f.img, nil
}
func (f *fakeImages) DeleteCourseChatImage(_ context.Context, id int64) error {
	*f.order = append(*f.order, "db")
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeStorage struct {
	repository.StorageRepository
	err     error
	copyErr error
	order   *[]string
	deleted []string
	copied  [][2]string
}

func (f *fakeStorage) StatObject(context.Context, string) (repository.ObjectInfo, error) {
	return repository.ObjectInfo{ETag: "etag"}, nil
}

func (f *fakeStorage) CopyObject(_ context.Context, src, dst, etag, _ string) error {
	*f.order = append(*f.order, "copy")
	if etag != "etag" {
		return errors.New("unexpected etag")
	}
	f.copied = append(f.copied, [2]string{src, dst})
	return f.copyErr
}

type fakeReports struct {
	repository.ReportedMediaRepository
	referenced bool
	repointErr error
	order      *[]string
	repointed  [][2]string
	targetType model.ReportTargetType
	targetID   string
}

func (f *fakeReports) HasReportedMedia(_ context.Context, targetType model.ReportTargetType, targetID, _ string) (bool, error) {
	f.targetType, f.targetID = targetType, targetID
	return f.referenced, nil
}

func (f *fakeReports) RepointReportedMedia(_ context.Context, _ model.ReportTargetType, _, oldKey, newKey string) error {
	*f.order = append(*f.order, "repoint")
	if f.repointErr != nil {
		return f.repointErr
	}
	f.repointed = append(f.repointed, [2]string{oldKey, newKey})
	return nil
}

type fakeParents struct {
	questionID int64
	answerID   int64
	messageID  int64
	roomID     int64
	loadErr    error
}

func (f *fakeParents) GetMessageByID(_ context.Context, id int64) (*model.Message, error) {
	return &model.Message{ID: id, RoomID: 3, UserID: 8}, f.loadErr
}
func (f *fakeParents) GetQuestionByID(_ context.Context, id int64) (*model.Question, error) {
	return &model.Question{ID: id}, f.loadErr
}
func (f *fakeParents) GetAnswerByID(_ context.Context, id int64) (*model.Answer, error) {
	return &model.Answer{ID: id}, f.loadErr
}
func (f *fakeParents) MessageUpdated(_ context.Context, ev chatusecase.MessageUpdatedEvent) {
	f.messageID, f.roomID = ev.Message.ID, ev.Room.ID
}
func (f *fakeParents) QuestionCreated(context.Context, *model.Question) {}
func (f *fakeParents) QuestionUpdated(_ context.Context, q *model.Question) {
	f.questionID = q.ID
}
func (f *fakeParents) QuestionDeleted(context.Context, *model.Question) {}
func (f *fakeParents) AnswerCreated(context.Context, *model.Answer)     {}
func (f *fakeParents) AnswerUpdated(_ context.Context, a *model.Answer) { f.answerID = a.ID }
func (f *fakeParents) AnswerDeleted(context.Context, *model.Answer)     {}

type fakeRooms struct{}

func (fakeRooms) Execute(_ context.Context, id int64) (*model.Room, error) {
	return &model.Room{ID: id}, nil
}

func newService(fi *fakeImages, reports *fakeReports, public, messages *fakeStorage, parents *fakeParents) *Service {
	return NewService(Deps{
		Images: fi, Reports: reports, Public: public, MessageStore: messages,
		Messages: parents, Questions: parents, Answers: parents, GetRoom: fakeRooms{},
		ChatEvents: parents, QuestionEvents: parents, AnswerEvents: parents,
	})
}

func (f *fakeStorage) DeleteObject(_ context.Context, key string) error {
	*f.order = append(*f.order, "storage")
	f.deleted = append(f.deleted, key)
	return f.err
}

func ctxAs(role string) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: role})
}

func setup(storageErr error, img *model.CourseChatImage) (*Service, *fakeImages, *[]string) {
	order := &[]string{}
	fi := &fakeImages{img: img, order: order}
	store := &fakeStorage{err: storageErr, order: order}
	return newService(fi, &fakeReports{order: order}, store, store, &fakeParents{}), fi, order
}

func sample() *model.CourseChatImage {
	return &model.CourseChatImage{Media: model.Media{ID: 5, StorageKey: "media/k.png"}, Source: model.CourseChatImageSourceQuestion, TargetID: 9}
}

func TestDelete_RemovesTheFileBeforeTheRecord(t *testing.T) {
	svc, fi, order := setup(nil, sample())
	if err := svc.Delete(ctxAs("administrator"), 5); err != nil {
		t.Fatal(err)
	}
	if len(*order) != 2 || (*order)[0] != "storage" || (*order)[1] != "db" || fi.deleted[0] != 5 {
		t.Errorf("order = %v", *order)
	}
}

func TestDelete_KeepsTheRecordWhenTheFileCannotBeDeleted(t *testing.T) {
	svc, fi, _ := setup(errors.New("storage down"), sample())
	if err := svc.Delete(ctxAs("administrator"), 5); err == nil {
		t.Fatal("want an error")
	}
	if len(fi.deleted) != 0 {
		t.Error("the record must stay so the deletion can be retried")
	}
}

func TestDelete_RejectsNonAdminAndNonChatImages(t *testing.T) {
	svc, fi, _ := setup(nil, sample())
	if err := svc.Delete(ctxAs("user"), 5); err == nil {
		t.Error("a non-admin must be rejected")
	}
	svc2, fi2, _ := setup(nil, nil)
	if err := svc2.Delete(ctxAs("administrator"), 6); err == nil || len(fi2.deleted) != 0 {
		t.Error("media that is not a course chat image must not be deletable here")
	}
	if len(fi.deleted) != 0 {
		t.Error("nothing may be deleted for a non-admin")
	}
}

func TestDelete_RemovesTheFileFromTheStoreThatHoldsIt(t *testing.T) {
	const uuid = "01234567-89ab-4cde-8f01-23456789abcd"
	messageKey, publicKey := "message-media/1/"+uuid+".png", "media/1/"+uuid+".png"
	for name, tc := range map[string]struct {
		source     model.CourseChatImageSource
		key        string
		wantPublic bool
	}{
		"message":             {model.CourseChatImageSourceMessage, messageKey, false},
		"message before move": {model.CourseChatImageSourceMessage, publicKey, true},
		"question":            {model.CourseChatImageSourceQuestion, publicKey, true},
	} {
		order := &[]string{}
		public, messages := &fakeStorage{order: order}, &fakeStorage{order: order}
		img := &model.CourseChatImage{Media: model.Media{ID: 5, StorageKey: tc.key}, Source: tc.source}
		svc := newService(&fakeImages{img: img, order: order}, &fakeReports{order: order}, public, messages, &fakeParents{})
		if err := svc.Delete(ctxAs("administrator"), 5); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, other := messages, public
		if tc.wantPublic {
			want, other = public, messages
		}
		if len(want.deleted) != 1 || len(other.deleted) != 0 {
			t.Errorf("%s: public=%v messages=%v", name, public.deleted, messages.deleted)
		}
	}
}

func TestDelete_RetainsAReportedImageUnderANewKeyBeforeDeletingIt(t *testing.T) {
	order := &[]string{}
	fi := &fakeImages{img: sample(), order: order}
	store := &fakeStorage{order: order}
	reports := &fakeReports{referenced: true, order: order}
	svc := newService(fi, reports, store, store, &fakeParents{})
	if err := svc.Delete(ctxAs("administrator"), 5); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*order, ","); got != "copy,repoint,storage,db" {
		t.Errorf("order = %s", got)
	}
	if reports.targetType != model.TargetQuestion || reports.targetID != "9" {
		t.Errorf("target = %s %s", reports.targetType, reports.targetID)
	}
	retained := store.copied[0][1]
	if retained == "media/k.png" || !strings.HasPrefix(retained, "media/") || !strings.HasSuffix(retained, ".png") {
		t.Errorf("retained key = %s", retained)
	}
	if reports.repointed[0] != [2]string{"media/k.png", retained} {
		t.Errorf("repointed = %v", reports.repointed)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "media/k.png" {
		t.Errorf("deleted = %v", store.deleted)
	}
}

func TestDelete_KeepsTheImageWhenItCannotBeRetained(t *testing.T) {
	for name, tweak := range map[string]func(*fakeStorage, *fakeReports){
		"copy":    func(s *fakeStorage, _ *fakeReports) { s.copyErr = errors.New("down") },
		"repoint": func(_ *fakeStorage, r *fakeReports) { r.repointErr = errors.New("down") },
	} {
		order := &[]string{}
		fi := &fakeImages{img: sample(), order: order}
		store := &fakeStorage{order: order}
		reports := &fakeReports{referenced: true, order: order}
		tweak(store, reports)
		svc := newService(fi, reports, store, store, &fakeParents{})
		if err := svc.Delete(ctxAs("administrator"), 5); err == nil {
			t.Fatalf("%s: want an error", name)
		}
		for _, key := range store.deleted {
			if key == "media/k.png" {
				t.Errorf("%s: the original must stay", name)
			}
		}
		if len(fi.deleted) != 0 {
			t.Errorf("%s: the record must stay", name)
		}
	}
}

func TestDelete_BroadcastsTheParentOfTheImage(t *testing.T) {
	for _, source := range []model.CourseChatImageSource{
		model.CourseChatImageSourceMessage, model.CourseChatImageSourceQuestion, model.CourseChatImageSourceAnswer,
	} {
		order := &[]string{}
		img := &model.CourseChatImage{Media: model.Media{ID: 5, StorageKey: "media/k.png"}, Source: source, TargetID: 9}
		store := &fakeStorage{order: order}
		parents := &fakeParents{}
		svc := newService(&fakeImages{img: img, order: order}, &fakeReports{order: order}, store, store, parents)
		if err := svc.Delete(ctxAs("administrator"), 5); err != nil {
			t.Fatal(err)
		}
		got := map[model.CourseChatImageSource]int64{
			model.CourseChatImageSourceMessage:  parents.messageID,
			model.CourseChatImageSourceQuestion: parents.questionID,
			model.CourseChatImageSourceAnswer:   parents.answerID,
		}
		if got[source] != 9 {
			t.Errorf("%s: not broadcast: %+v", source, parents)
		}
		if source == model.CourseChatImageSourceMessage && parents.roomID != 3 {
			t.Errorf("room = %d", parents.roomID)
		}
	}
}

func TestDelete_SucceedsEvenIfTheParentCannotBeLoaded(t *testing.T) {
	order := &[]string{}
	store := &fakeStorage{order: order}
	fi := &fakeImages{img: sample(), order: order}
	svc := newService(fi, &fakeReports{order: order}, store, store, &fakeParents{loadErr: errors.New("gone")})
	if err := svc.Delete(ctxAs("administrator"), 5); err != nil {
		t.Fatal(err)
	}
	if len(fi.deleted) != 1 {
		t.Error("the record should be deleted")
	}
}
