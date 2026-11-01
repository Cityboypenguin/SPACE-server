package graph

import (
	"context"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// headerStore は UploadRequestHeaders だけを差し替えられる置き場。
// 他のメソッドは呼ばれないので、呼ばれたら失敗させる。
type headerStore struct {
	headers map[string]string
	t       *testing.T
}

func (s headerStore) UploadRequestHeaders() map[string]string { return s.headers }

func (s headerStore) PresignedPutURL(context.Context, string, string, time.Duration, int64) (string, error) {
	s.t.Fatal("PresignedPutURL は呼ばれないはず")
	return "", nil
}
func (s headerStore) StatObject(context.Context, string) (repository.ObjectInfo, error) {
	s.t.Fatal("StatObject は呼ばれないはず")
	return repository.ObjectInfo{}, nil
}
func (s headerStore) CopyObject(context.Context, string, string, string, string) error {
	s.t.Fatal("CopyObject は呼ばれないはず")
	return nil
}
func (s headerStore) ObjectHead(context.Context, string, int64) ([]byte, string, error) {
	s.t.Fatal("ObjectHead は呼ばれないはず")
	return nil, "", nil
}
func (s headerStore) PublicURL(string) string {
	s.t.Fatal("PublicURL は呼ばれないはず")
	return ""
}
func (s headerStore) DeleteObject(context.Context, string) error {
	s.t.Fatal("DeleteObject は呼ばれないはず")
	return nil
}

var _ repository.StorageRepository = headerStore{}

// 置き場が要求したヘッダーが、名前順で、そのまま並ぶこと。
//
// 順を固定するのは、map の走査順が毎回変わるため。揺れると同じ要求に対する
// 応答が毎回違うものになる。
func TestUploadHeadersAreSortedByName(t *testing.T) {
	store := headerStore{t: t, headers: map[string]string{
		"x-ms-blob-type": "BlockBlob",
		"a-first":        "1",
		"m-middle":       "2",
	}}

	got := uploadHeaders(store)
	if len(got) != 3 {
		t.Fatalf("ヘッダーが %d 個になった（3 個のはず）", len(got))
	}

	wantNames := []string{"a-first", "m-middle", "x-ms-blob-type"}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Errorf("%d 番目が %q（%q のはず）", i, got[i].Name, want)
		}
		if got[i].Value != store.headers[want] {
			t.Errorf("%s の値が %q（%q のはず）", want, got[i].Value, store.headers[want])
		}
	}
}

// ヘッダーを要求しない置き場では、空の配列になること（null ではない）。
//
// GraphQL の型が [UploadHeader!]! なので null は返せず、クライアントは
// そのまま for で回せる。
func TestUploadHeadersIsEmptyWhenNoneAreRequired(t *testing.T) {
	for _, headers := range []map[string]string{nil, {}} {
		got := uploadHeaders(headerStore{t: t, headers: headers})
		if got == nil {
			t.Error("nil が返った（空の配列のはず）")
		}
		if len(got) != 0 {
			t.Errorf("%d 個返った（0 個のはず）", len(got))
		}
	}
}
