package upload

import (
	"context"
	"strings"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeStore は StatObject が返す内容を決め打ちし、消されたキーを覚えておく。
type fakeStore struct {
	info    repository.ObjectInfo
	statErr error
	deleted []string
}

func (f *fakeStore) StatObject(_ context.Context, _ string) (repository.ObjectInfo, error) {
	return f.info, f.statErr
}

func (f *fakeStore) DeleteObject(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

// TestVerify_RejectsOversizedObjects は項目5の本体。
//
// 署名付きURLでは上限を強制できないので、URLを受け取った利用者は上限を超える
// オブジェクトをそのまま置ける。受け入れ時に実物を測って弾けていないと、
// 「20MBまで」と言いながら何GBでも保存できる状態になる。
func TestVerify_RejectsOversizedObjects(t *testing.T) {
	store := &fakeStore{info: repository.ObjectInfo{
		Size:        Media.MaxBytes + 1,
		ContentType: "image/png",
	}}

	err := Verify(context.Background(), store, "media/42/x.png")
	if err == nil {
		t.Fatal("上限を超えたオブジェクトを受け入れてしまった")
	}
	if !strings.Contains(err.Error(), "上限") {
		t.Fatalf("error = %v, want サイズ上限の説明", err)
	}
	// 受け入れを拒んだオブジェクトはもう参照されないので、残す理由が無い。
	if len(store.deleted) != 1 || store.deleted[0] != "media/42/x.png" {
		t.Fatalf("deleted = %v, want 違反したオブジェクトが消えていること", store.deleted)
	}
}

func TestVerify_AcceptsObjectsWithinTheLimit(t *testing.T) {
	store := &fakeStore{info: repository.ObjectInfo{
		Size:        Media.MaxBytes,
		ContentType: "image/png",
	}}

	if err := Verify(context.Background(), store, "media/42/x.png"); err != nil {
		t.Fatalf("上限ちょうどは受け入れること: %v", err)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want 受け入れたものは消さない", store.deleted)
	}
}

// TestVerify_RejectsUnexpectedContentTypes は、サイズだけ見ていると素通りする
// 「画像の口に置かれた実行可能ファイル」を弾けることを確かめる。
// 公開URLからそのまま配られるので、種別も見る必要がある。
func TestVerify_RejectsUnexpectedContentTypes(t *testing.T) {
	store := &fakeStore{info: repository.ObjectInfo{
		Size:        1024,
		ContentType: "application/x-msdownload",
	}}

	if err := Verify(context.Background(), store, "avatars/42/x.png"); err == nil {
		t.Fatal("画像以外を受け入れてしまった")
	}
	if len(store.deleted) != 1 {
		t.Fatalf("deleted = %v, want 違反したオブジェクトが消えていること", store.deleted)
	}
}

// TestVerify_RejectsUnknownPrefixes は、こちらが署名付きURLを出していない
// キーを申告された場合を確かめる。種別が決まらない＝上限も決まらないので、
// 通してはいけない。
func TestVerify_RejectsUnknownPrefixes(t *testing.T) {
	store := &fakeStore{info: repository.ObjectInfo{Size: 1, ContentType: "image/png"}}

	for _, key := range []string{"private/secret.md", "x.png", "", "../etc/passwd"} {
		if err := Verify(context.Background(), store, key); err == nil {
			t.Fatalf("未知のキー %q を受け入れてしまった", key)
		}
	}
}

// TestVerify_FailsClosedWhenStorageIsDown は、実物を測れなかったときに
// 通してしまわないことを確かめる。ここが素通しになると、ストレージ障害中だけ
// 上限が消える。
func TestVerify_FailsClosedWhenStorageIsDown(t *testing.T) {
	store := &fakeStore{statErr: context.DeadlineExceeded}

	if err := Verify(context.Background(), store, "media/42/x.png"); err == nil {
		t.Fatal("実物を測れないまま受け入れてしまった")
	}
	// 測れていない以上、消してもいけない（上限内の正当なオブジェクトかもしれない）。
	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want 測れなかったものは消さない", store.deleted)
	}
}

// TestVerify_ReportsMissingObjectsDistinctly は、まだ置かれていないキーを
// 申告された場合に、その旨が伝わることを確かめる。
func TestVerify_ReportsMissingObjectsDistinctly(t *testing.T) {
	store := &fakeStore{statErr: repository.ErrObjectNotFound}

	err := Verify(context.Background(), store, "media/42/x.png")
	if err == nil {
		t.Fatal("存在しないキーを受け入れてしまった")
	}
	if !strings.Contains(err.Error(), "アップロードされていない") {
		t.Fatalf("error = %v, want 未アップロードの説明", err)
	}
}

// TestKindLimitsMatchTheDocumentedValues は、上限表がAPIの約束どおりであることを
// 留める。ここを変えるときはクライアントの表示も一緒に変わる必要がある。
func TestKindLimitsMatchTheDocumentedValues(t *testing.T) {
	cases := []struct {
		kind Kind
		want int64
	}{
		{Avatar, 5 * mb},
		{Media, 20 * mb},
		{CommunityIcon, 5 * mb},
		{TermsDocument, 5 * mb},
	}
	for _, c := range cases {
		if c.kind.MaxBytes != c.want {
			t.Errorf("%s の上限 = %d, want %d", c.kind.Prefix, c.kind.MaxBytes, c.want)
		}
	}
}

// TestKindForObjectKey_CoversEveryPresignPrefix は、URLを出す側の接頭辞が
// すべて引けることを確かめる。引けない接頭辞があると、その種別のアップロードが
// 受け入れ時に必ず弾かれる（URLは出るのに保存できない、という形で出る）。
func TestKindForObjectKey_CoversEveryPresignPrefix(t *testing.T) {
	for _, k := range kinds {
		got, ok := KindForObjectKey(k.Prefix + "/123/x" + firstExt(k))
		if !ok {
			t.Fatalf("%s が引けない", k.Prefix)
		}
		if got.Prefix != k.Prefix {
			t.Fatalf("prefix %q -> %q", k.Prefix, got.Prefix)
		}
	}
}

func firstExt(k Kind) string {
	for _, ext := range k.Exts {
		return ext
	}
	return ""
}
