package reportedmedia

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type fakeReports struct {
	repository.ReportedMediaRepository
	reported  map[string]bool
	repointed [][2]string
	err       error
}

func (f *fakeReports) HasReportedMedia(_ context.Context, _ model.ReportTargetType, _, key string) (bool, error) {
	return f.reported[key], nil
}

func (f *fakeReports) RepointReportedMedia(_ context.Context, _ model.ReportTargetType, _, oldKey, newKey string) error {
	f.repointed = append(f.repointed, [2]string{oldKey, newKey})
	return f.err
}

type fakeStore struct {
	repository.StorageRepository
	missing bool
	putErr  error
	put     map[string]string
}

func (f *fakeStore) StatObject(context.Context, string) (repository.ObjectInfo, error) {
	if f.missing {
		return repository.ObjectInfo{}, repository.ErrObjectNotFound
	}
	return repository.ObjectInfo{Size: 3}, nil
}

func (f *fakeStore) ObjectHead(context.Context, string, int64) ([]byte, string, error) {
	return []byte("png"), "etag", nil
}

func (f *fakeStore) PutObject(_ context.Context, key, _ string, body []byte) error {
	if f.put == nil {
		f.put = map[string]string{}
	}
	f.put[key] = string(body)
	return f.putErr
}

const reportedKey = "media/4d6c8d02-3a35-4a52-a1a1-2b3c4d5e6f70.png"

func item(id int64, key string, target model.ReportTargetType) repository.ReportableMedia {
	return repository.ReportableMedia{MediaID: id, StorageKey: key, ContentType: "image/png", TargetType: target, TargetID: 9}
}

func TestRetain_CopiesOnlyReportedMediaToThePrivateStore(t *testing.T) {
	reports := &fakeReports{reported: map[string]bool{reportedKey: true}}
	public, private := &fakeStore{}, &fakeStore{}
	r := New(reports, public, private)
	err := r.Retain(context.Background(), []repository.ReportableMedia{
		item(5, reportedKey, model.TargetQuestion),
		item(6, "media/other.png", model.TargetQuestion),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(private.put) != 1 || private.put["removed/5.png"] != "png" || len(public.put) != 0 {
		t.Errorf("public=%v private=%v", public.put, private.put)
	}
	if len(reports.repointed) != 1 || reports.repointed[0] != [2]string{reportedKey, "removed/5.png"} {
		t.Errorf("repointed = %v", reports.repointed)
	}
}

func TestRetain_SkipsMissingFiles(t *testing.T) {
	reports := &fakeReports{reported: map[string]bool{reportedKey: true}}
	r := New(reports, &fakeStore{missing: true}, &fakeStore{})
	if err := r.Retain(context.Background(), []repository.ReportableMedia{item(5, reportedKey, model.TargetQuestion)}); err != nil {
		t.Fatal(err)
	}
	if len(reports.repointed) != 0 {
		t.Errorf("repointed = %v", reports.repointed)
	}
}

func TestRetain_ReturnsFailuresSoThePurgeIsRetried(t *testing.T) {
	down := errors.New("down")
	for name, r := range map[string]*Retainer{
		"put":     New(&fakeReports{reported: map[string]bool{reportedKey: true}}, &fakeStore{}, &fakeStore{putErr: down}),
		"repoint": New(&fakeReports{reported: map[string]bool{reportedKey: true}, err: down}, &fakeStore{}, &fakeStore{}),
	} {
		if err := r.Retain(context.Background(), []repository.ReportableMedia{item(5, reportedKey, model.TargetQuestion)}); !errors.Is(err, down) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
