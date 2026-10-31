package courseimage

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
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
	err   error
	order *[]string
}

func (f *fakeStorage) DeleteObject(context.Context, string) error {
	*f.order = append(*f.order, "storage")
	return f.err
}

func ctxAs(role string) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: role})
}

func setup(storageErr error, img *model.CourseChatImage) (*Service, *fakeImages, *[]string) {
	order := &[]string{}
	fi := &fakeImages{img: img, order: order}
	return NewService(fi, &fakeStorage{err: storageErr, order: order}), fi, order
}

func sample() *model.CourseChatImage {
	return &model.CourseChatImage{Media: model.Media{ID: 5, StorageKey: "k"}, Source: model.CourseChatImageSourceQuestion, TargetID: 9}
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
