package courseimage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
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

	removed         *model.RemovedCourseChatImage
	expired         []*model.RemovedCourseChatImage
	purged          []int64
	before          time.Time
	restoreCheckErr error
	restoreErr      error
}

func (f *fakeImages) GetCourseChatImage(context.Context, int64) (*model.CourseChatImage, error) {
	return f.img, nil
}
func (f *fakeImages) RemoveCourseChatImage(_ context.Context, removed *model.RemovedCourseChatImage) error {
	*f.order = append(*f.order, "db")
	f.deleted = append(f.deleted, removed.MediaID)
	f.removed = removed
	return nil
}
func (f *fakeImages) GetRemovedCourseChatImage(context.Context, int64) (*model.RemovedCourseChatImage, error) {
	return f.removed, nil
}
func (f *fakeImages) ListRemovedCourseChatImagesBefore(_ context.Context, before time.Time, after int64, _ int) ([]*model.RemovedCourseChatImage, error) {
	f.before = before
	var out []*model.RemovedCourseChatImage
	for _, img := range f.expired {
		if img.MediaID > after {
			out = append(out, img)
		}
	}
	return out, nil
}
func (f *fakeImages) CheckRestorable(context.Context, *model.RemovedCourseChatImage) error {
	return f.restoreCheckErr
}
func (f *fakeImages) RestoreCourseChatImage(context.Context, *model.RemovedCourseChatImage) error {
	*f.order = append(*f.order, "restore")
	return f.restoreErr
}
func (f *fakeImages) DeleteRemovedCourseChatImage(_ context.Context, id int64) error {
	f.purged = append(f.purged, id)
	return nil
}

type fakeStorage struct {
	repository.StorageRepository
	err     error
	putErr  error
	statErr error
	order   *[]string
	deleted []string
	put     map[string]string
}

func (f *fakeStorage) StatObject(context.Context, string) (repository.ObjectInfo, error) {
	return repository.ObjectInfo{Size: 3}, f.statErr
}

func (f *fakeStorage) ObjectHead(_ context.Context, _ string, limit int64) ([]byte, string, error) {
	if limit != 3 {
		return nil, "", errors.New("must read the whole object")
	}
	return []byte("png"), "etag", nil
}

func (f *fakeStorage) PutObject(_ context.Context, key, _ string, body []byte) error {
	*f.order = append(*f.order, "put")
	if f.put == nil {
		f.put = map[string]string{}
	}
	f.put[key] = string(body)
	return f.putErr
}

func (f *fakeStorage) DeleteObject(_ context.Context, key string) error {
	*f.order = append(*f.order, "storage")
	f.deleted = append(f.deleted, key)
	return f.err
}

type fakeReports struct {
	repository.ReportedMediaRepository
	repointErr error
	order      *[]string
	repointed  [][2]string
	targetType model.ReportTargetType
	targetID   string
	reported   map[string]bool
}

func (f *fakeReports) HasReportedMedia(_ context.Context, _ model.ReportTargetType, _, key string) (bool, error) {
	return f.reported[key], nil
}

func (f *fakeReports) RepointReportedMedia(_ context.Context, targetType model.ReportTargetType, targetID, oldKey, newKey string) error {
	*f.order = append(*f.order, "repoint")
	f.targetType, f.targetID = targetType, targetID
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

func TestDelete_MovesTheFileOutOfReachBeforeTheRecord(t *testing.T) {
	order := &[]string{}
	fi := &fakeImages{img: sample(), order: order}
	public, messages := &fakeStorage{order: order}, &fakeStorage{order: order}
	reports := &fakeReports{order: order}
	svc := newService(fi, reports, public, messages, &fakeParents{})
	if err := svc.Delete(ctxAs("administrator"), 5); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*order, ","); got != "put,repoint,storage,db,repoint" {
		t.Errorf("order = %s", got)
	}
	if len(public.put) != 0 || len(messages.put) != 1 {
		t.Fatalf("must go to the private store: public=%v messages=%v", public.put, messages.put)
	}
	var removedKey string
	for key, body := range messages.put {
		removedKey = key
		if key != "removed/5.png" || body != "png" {
			t.Errorf("put %q = %q", key, body)
		}
	}
	if reports.targetType != model.TargetQuestion || reports.targetID != "9" {
		t.Errorf("target = %s %s", reports.targetType, reports.targetID)
	}
	if reports.repointed[0] != [2]string{"media/k.png", removedKey} {
		t.Errorf("repointed = %v", reports.repointed)
	}
	if len(public.deleted) != 1 || public.deleted[0] != "media/k.png" || fi.deleted[0] != 5 {
		t.Errorf("deleted file=%v record=%v", public.deleted, fi.deleted)
	}
	if fi.removed.RemovedKey != removedKey || fi.removed.OriginalKey != "media/k.png" || fi.removed.RemovedBy != 1 {
		t.Errorf("removed = %+v", fi.removed)
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

func TestDelete_KeepsTheOriginalWhenItCannotBeMoved(t *testing.T) {
	for name, tweak := range map[string]func(*fakeStorage, *fakeStorage, *fakeReports){
		"stat":    func(p, _ *fakeStorage, _ *fakeReports) { p.statErr = errors.New("down") },
		"put":     func(_, m *fakeStorage, _ *fakeReports) { m.putErr = errors.New("down") },
		"repoint": func(_, _ *fakeStorage, r *fakeReports) { r.repointErr = errors.New("down") },
	} {
		order := &[]string{}
		fi := &fakeImages{img: sample(), order: order}
		public, messages := &fakeStorage{order: order}, &fakeStorage{order: order}
		reports := &fakeReports{order: order}
		tweak(public, messages, reports)
		svc := newService(fi, reports, public, messages, &fakeParents{})
		if err := svc.Delete(ctxAs("administrator"), 5); err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if len(public.deleted) != 0 || len(fi.deleted) != 0 {
			t.Errorf("%s: the original and the record must stay", name)
		}
	}
}

func TestDelete_RetriesWhenTheFileIsAlreadyGone(t *testing.T) {
	order := &[]string{}
	fi := &fakeImages{img: sample(), order: order}
	public, messages := &fakeStorage{order: order, statErr: repository.ErrObjectNotFound}, &fakeStorage{order: order}
	svc := newService(fi, &fakeReports{order: order}, public, messages, &fakeParents{})
	if err := svc.Delete(ctxAs("administrator"), 5); err != nil {
		t.Fatal(err)
	}
	if len(messages.put) != 0 || len(fi.deleted) != 1 {
		t.Errorf("put=%v record=%v", messages.put, fi.deleted)
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

func removedSample() *model.RemovedCourseChatImage {
	return &model.RemovedCourseChatImage{
		MediaID: 5, Source: model.CourseChatImageSourceQuestion, TargetID: 9,
		RemovedKey: "removed/5.png", OriginalKey: "media/k.png", ContentType: "image/png",
	}
}

func TestRestore_PutsTheFileBackThenDropsTheRemovedCopy(t *testing.T) {
	order := &[]string{}
	fi := &fakeImages{img: sample(), order: order, removed: removedSample()}
	public, messages := &fakeStorage{order: order}, &fakeStorage{order: order}
	reports := &fakeReports{order: order}
	parents := &fakeParents{}
	svc := newService(fi, reports, public, messages, parents)
	if err := svc.Restore(ctxAs("administrator"), 5); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*order, ","); got != "put,restore,repoint,storage" {
		t.Errorf("order = %s", got)
	}
	if public.put["media/k.png"] != "png" {
		t.Errorf("must go back to the original store: %v", public.put)
	}
	if reports.repointed[0] != [2]string{"removed/5.png", "media/k.png"} {
		t.Errorf("repointed = %v", reports.repointed)
	}
	if len(messages.deleted) != 1 || messages.deleted[0] != "removed/5.png" {
		t.Errorf("deleted = %v", messages.deleted)
	}
	if parents.questionID != 9 {
		t.Error("the parent should be broadcast")
	}
}

func TestRestore_RejectedWhenThePostOrUploaderIsGone(t *testing.T) {
	for _, cause := range []error{repository.ErrRestoreTargetGone, repository.ErrRestoreUploaderGone} {
		order := &[]string{}
		fi := &fakeImages{order: order, removed: removedSample(), restoreCheckErr: cause}
		store := &fakeStorage{order: order}
		svc := newService(fi, &fakeReports{order: order}, store, store, &fakeParents{})
		err := svc.Restore(ctxAs("administrator"), 5)
		if apperr.CodeOf(err) != apperr.CodeConflict {
			t.Errorf("%v: err = %v", cause, err)
		}
		if len(*order) != 0 {
			t.Errorf("%v: nothing should be touched: %v", cause, *order)
		}
	}
}

func TestRestore_RemovesThePutBackFileWhenTheRecordCannotBeRestored(t *testing.T) {
	order := &[]string{}
	fi := &fakeImages{order: order, removed: removedSample(), restoreErr: errors.New("down")}
	public, messages := &fakeStorage{order: order}, &fakeStorage{order: order}
	svc := newService(fi, &fakeReports{order: order}, public, messages, &fakeParents{})
	if err := svc.Restore(ctxAs("administrator"), 5); err == nil {
		t.Fatal("want an error")
	}
	if len(public.deleted) != 1 || public.deleted[0] != "media/k.png" || len(messages.deleted) != 0 {
		t.Errorf("public=%v messages=%v", public.deleted, messages.deleted)
	}
}

func TestRestore_RequiresAdmin(t *testing.T) {
	svc, _, _ := setup(nil, sample())
	if err := svc.Restore(ctxAs("user"), 5); err == nil {
		t.Fatal("want an error")
	}
}

func TestPurgeExpired_SkipsReportedImages(t *testing.T) {
	order := &[]string{}
	unreported, reported := removedSample(), removedSample()
	reported.MediaID, reported.RemovedKey = 6, "removed/6.png"
	fi := &fakeImages{order: order, expired: []*model.RemovedCourseChatImage{unreported, reported}}
	store := &fakeStorage{order: order}
	svc := newService(fi, &fakeReports{order: order, reported: map[string]bool{"removed/6.png": true}}, store, store, &fakeParents{})
	now := time.Unix(1800000000, 0)
	purged, err := svc.PurgeExpired(context.Background(), now)
	if err != nil || purged != 1 {
		t.Fatalf("purged=%d err=%v", purged, err)
	}
	if !fi.before.Equal(now.Add(-30 * 24 * time.Hour)) {
		t.Errorf("before = %v", fi.before)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "removed/5.png" || len(fi.purged) != 1 || fi.purged[0] != 5 {
		t.Errorf("files=%v records=%v", store.deleted, fi.purged)
	}
}

func TestPurgeExpired_KeepsTheRecordWhenTheFileCannotBeDeleted(t *testing.T) {
	order := &[]string{}
	fi := &fakeImages{order: order, expired: []*model.RemovedCourseChatImage{removedSample()}}
	store := &fakeStorage{order: order, err: errors.New("down")}
	svc := newService(fi, &fakeReports{order: order}, store, store, &fakeParents{})
	purged, err := svc.PurgeExpired(context.Background(), time.Unix(1800000000, 0))
	if err == nil || purged != 0 || len(fi.purged) != 0 {
		t.Errorf("purged=%d err=%v records=%v", purged, err, fi.purged)
	}
}
