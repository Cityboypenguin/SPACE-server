package upload

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

// fakeStore は「在るオブジェクト」を明示して持つ ObjectStore。
//
// 在る／在らないを決め打ちにせず表で持つのは、受け入れの判断が
// **最終キーが在るかどうか**で変わるようになったため（Accept のコメント参照）。
// 「何でも在る」偽物だと、受け入れが常に「公開済み」に倒れて何も確かめられない。
type fakeStore struct {
	mu      sync.Mutex
	objects map[string]repository.ObjectInfo
	statErr error
	copyErr error
	deleted []string
	copied  []copiedObject
	// writes は最終キーごとの書き込み回数。公開済みのキーが二度書かれないことを
	// 見るために要る（2回目の中身が1回目と同じとは限らないので、結果だけ見ても
	// 分からない）。
	writes map[string]int
}

type copiedObject struct {
	src, dst, etag string
}

// newFakeStore は keys に挙げたキーだけが在る置き場を作る。
func newFakeStore(info repository.ObjectInfo, keys ...string) *fakeStore {
	objects := make(map[string]repository.ObjectInfo, len(keys))
	for _, k := range keys {
		objects[k] = info
	}
	return &fakeStore{objects: objects, writes: map[string]int{}}
}

func (f *fakeStore) StatObject(_ context.Context, key string) (repository.ObjectInfo, error) {
	if f.statErr != nil {
		return repository.ObjectInfo{}, f.statErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	info, ok := f.objects[key]
	if !ok {
		return repository.ObjectInfo{}, repository.ErrObjectNotFound
	}
	return info, nil
}

func (f *fakeStore) CopyObject(_ context.Context, src, dst, etag string) error {
	if f.copyErr != nil {
		return f.copyErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.copied = append(f.copied, copiedObject{src, dst, etag})
	f.writes[dst]++
	f.objects[dst] = f.objects[src]
	return nil
}

func (f *fakeStore) DeleteObject(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, key)
	delete(f.objects, key)
	return nil
}

// 鍵は「こちらが払い出した形」でなければ通らないので、テストでも実際に作る。
func mediaKey(t *testing.T) string {
	t.Helper()
	key, err := Media.NewStagingKey("42", "image/png")
	if err != nil {
		t.Fatalf("NewStagingKey: %v", err)
	}
	return key
}

// 検査に通る画像1枚ぶんの実寸。
func okImage() repository.ObjectInfo {
	return repository.ObjectInfo{Size: 1024, ContentType: "image/png", ETag: "etag-1"}
}

// TestVerify_RejectsOversizedObjects は項目5の本体。
//
// 署名付きURLでは上限を強制できないので、URLを受け取った利用者は上限を超える
// オブジェクトをそのまま置ける。受け入れ時に実物を測って弾けていないと、
// 「20MBまで」と言いながら何GBでも保存できる状態になる。
func TestVerify_RejectsOversizedObjects(t *testing.T) {
	key := acceptedForm(mediaKey(t))
	store := newFakeStore(repository.ObjectInfo{
		Size:        Media.MaxBytes + 1,
		ContentType: "image/png",
		ETag:        "etag-1",
	}, key)
	err := Verify(context.Background(), store, key)
	if err == nil {
		t.Fatal("上限を超えたオブジェクトを受け入れてしまった")
	}
	if !strings.Contains(err.Error(), "上限") {
		t.Fatalf("error = %v, want サイズ上限の説明", err)
	}
	// 受け入れを拒んだオブジェクトはもう参照されないので、残す理由が無い。
	if len(store.deleted) != 1 || store.deleted[0] != key {
		t.Fatalf("deleted = %v, want 違反したオブジェクトが消えていること", store.deleted)
	}
}

func TestVerify_AcceptsObjectsWithinTheLimit(t *testing.T) {
	key := acceptedForm(mediaKey(t))
	store := newFakeStore(repository.ObjectInfo{
		Size:        Media.MaxBytes,
		ContentType: "image/png",
		ETag:        "etag-1",
	}, key)

	if err := Verify(context.Background(), store, key); err != nil {
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
	key := acceptedForm(avatarKey(t))
	store := newFakeStore(repository.ObjectInfo{
		Size:        1024,
		ContentType: "application/x-msdownload",
		ETag:        "etag-1",
	}, key)

	if err := Verify(context.Background(), store, key); err == nil {
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
	store := newFakeStore(repository.ObjectInfo{Size: 1, ContentType: "image/png", ETag: "etag-1"})

	for _, key := range []string{"private/secret.md", "x.png", "", "../etc/passwd", "media/42/../../secret.png", "media/42/notauuid.png", "media/42/" + strings.Repeat("a", 8) + ".exe"} {
		if err := Verify(context.Background(), store, key); err == nil {
			t.Fatalf("未知のキー %q を受け入れてしまった", key)
		}
	}
}

// TestVerify_FailsClosedWhenStorageIsDown は、実物を測れなかったときに
// 通してしまわないことを確かめる。ここが素通しになると、ストレージ障害中だけ
// 上限が消える。
func TestVerify_FailsClosedWhenStorageIsDown(t *testing.T) {
	store := newFakeStore(okImage())
	store.statErr = context.DeadlineExceeded

	if err := Verify(context.Background(), store, acceptedForm(mediaKey(t))); err == nil {
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
	store := newFakeStore(okImage())

	err := Verify(context.Background(), store, acceptedForm(mediaKey(t)))
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
		key, err := k.NewStagingKey("123", firstContentType(k))
		if err != nil {
			t.Fatalf("%s: %v", k.Prefix, err)
		}
		got, ok := KindForObjectKey(key)
		if !ok {
			t.Fatalf("%s が引けない", k.Prefix)
		}
		if got.Prefix != k.Prefix {
			t.Fatalf("prefix %q -> %q", k.Prefix, got.Prefix)
		}
	}
}

func avatarKey(t *testing.T) string {
	t.Helper()
	key, err := Avatar.NewStagingKey("42", "image/png")
	if err != nil {
		t.Fatalf("NewStagingKey: %v", err)
	}
	return key
}

// acceptedForm は staging 接頭辞を外しただけの「受け入れ済みの形」のキー。
// 受け入れが実際に返すキー（名前は引き直される）ではなく、素通し経路や検査の
// テストで「staging でないキー」が要るときに使う。
func acceptedForm(key string) string {
	return strings.TrimPrefix(key, StagingPrefix+"/")
}

func firstContentType(k Kind) string {
	for ct := range k.Exts {
		return ct
	}
	return ""
}

// --- 検査後の差し替え（受け入れ時にキーを移す） ---------------------------

// TestAccept_MovesTheObjectOutOfReachOfTheSignedURL は今回の本体。
//
// 署名付きURLは有効期間のあいだ何度でも書ける。検査に通したキーをそのまま
// 保存すると、「規定内の画像を置く → 受け入れさせる → 同じURLで中身だけ
// 差し替える」で、参照されているものが検査済みでなくなる。
//
// 受け入れが返すのは、署名付きURLを一度も出していないキーでなければならない。
func TestAccept_MovesTheObjectOutOfReachOfTheSignedURL(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(okImage(), staging)

	got, err := Accept(context.Background(), store, staging, Media, "42")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	if strings.HasPrefix(got, StagingPrefix+"/") {
		t.Fatalf("受け入れたキー %q が staging のまま（署名付きURLで書き換えられる）", got)
	}
	// 写し先は staging の名前から導けないこと。導けると、同じ staging キーに
	// 対する2回目の受け入れが同じ場所へ書き直せてしまう。
	if got == acceptedForm(staging) {
		t.Fatalf("key = %q が staging の名前から導ける", got)
	}
	// 種別と所有者の区切りは引き継ぐこと（キーから持ち主を判定する経路がある）。
	dir := acceptedForm(staging)[:strings.LastIndex(acceptedForm(staging), "/")+1]
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("key = %q, want %q で始まること", got, dir)
	}
	if _, ok := KindForObjectKey(got); !ok {
		t.Fatalf("受け入れたキー %q が種別を引けない", got)
	}
	if len(store.copied) != 1 || store.copied[0] != (copiedObject{staging, got, "etag-1"}) {
		t.Fatalf("copied = %v, want %s -> %s", store.copied, staging, got)
	}
	// 写した後の staging は用済み。残すと、書き換えられる実体が転がり続ける。
	if len(store.deleted) != 1 || store.deleted[0] != staging {
		t.Fatalf("deleted = %v, want staging が消えていること", store.deleted)
	}
}

// 検査に落ちたものは写さないこと。写してしまうと、上限超過の実体が
// 「もう書き換えられない場所」に固定されるだけで、何も良くならない。
func TestAccept_DoesNotPromoteWhatItRejects(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(repository.ObjectInfo{
		Size:        Media.MaxBytes + 1,
		ContentType: "image/png",
		ETag:        "etag-1",
	}, staging)

	if _, err := Accept(context.Background(), store, staging, Media, "42"); err == nil {
		t.Fatal("上限を超えたオブジェクトを受け入れてしまった")
	}
	if len(store.copied) != 0 {
		t.Fatalf("copied = %v, want 検査に落ちたものは写さない", store.copied)
	}
}

// 写せなかったときに受け入れを成立させないこと。
// 成立させると、実体の無いキーが DB に残る。
func TestAccept_FailsWhenTheCopyFails(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(okImage(), staging)
	store.copyErr = context.DeadlineExceeded

	if _, err := Accept(context.Background(), store, staging, Media, "42"); err == nil {
		t.Fatal("写せていないのに受け入れてしまった")
	}
}

// 既に受け入れ済みのキー（編集で送り返されたもの）は、写さずにそのまま通すこと。
//
// 添付を変えずに投稿を編集する経路では、前回保存したキーがそのまま返ってくる。
// ここで写そうとすると staging に実体が無いので必ず失敗し、編集ができなくなる。
func TestAccept_PassesThroughAlreadyAcceptedKeys(t *testing.T) {
	final := acceptedForm(mediaKey(t))
	store := newFakeStore(okImage(), final)

	got, err := Accept(context.Background(), store, final, Media, "42")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if got != final {
		t.Fatalf("key = %q, want %q", got, final)
	}
	if len(store.copied) != 0 {
		t.Fatalf("copied = %v, want 受け入れ済みのキーは写さない", store.copied)
	}
}

// staging も写し先も無いなら、素直に「置かれていない」と答えること。
func TestAccept_ReportsMissingUploads(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(okImage())

	_, err := Accept(context.Background(), store, staging, Media, "42")
	if err == nil {
		t.Fatal("置かれていないキーを受け入れてしまった")
	}
	if !strings.Contains(err.Error(), "アップロードされていない") {
		t.Fatalf("error = %v, want 未アップロードの説明", err)
	}
}

// 署名付きURLを出すのは staging だけであること。
//
// ここが崩れると、最終キーに対する書き込みURLが存在することになり、
// 写す意味がなくなる。
func TestNewStagingKey_AlwaysStages(t *testing.T) {
	for _, k := range kinds {
		key, err := k.NewStagingKey("42", firstContentType(k))
		if err != nil {
			t.Fatalf("%s: %v", k.Prefix, err)
		}
		if !strings.HasPrefix(key, StagingPrefix+"/") {
			t.Fatalf("%s のキー %q が staging の外", k.Prefix, key)
		}
		// 出したキーは、そのまま受け入れ側で読めること。
		if _, ok := KindForObjectKey(key); !ok {
			t.Fatalf("%s のキー %q が受け入れ側で引けない", k.Prefix, key)
		}
	}
}

// 所有者の無い種別（規約ドキュメント）も往復すること。
func TestNewStagingKey_WorksWithoutAnOwnerSegment(t *testing.T) {
	key, err := TermsDocument.NewStagingKey("", "text/markdown")
	if err != nil {
		t.Fatalf("NewStagingKey: %v", err)
	}
	if _, ok := KindForObjectKey(key); !ok {
		t.Fatalf("キー %q が引けない", key)
	}
	if got := acceptedForm(key); strings.HasPrefix(got, StagingPrefix+"/") {
		t.Fatalf("final = %q", got)
	}
}

// --- 公開済みの実体は変えられないこと ---------------------------------------

// TestAccept_NeverWritesAPublishedKeyTwice は今回の本体。
//
// staging へ写しただけでは実体は固まらない。署名付きURLは有効な間ずっと書けるので、
// 受け入れさせた後に staging へ置き直して同じキーをもう一度申告できる。写し先が
// staging の名前から導ける作りだと、そこで公開済みの実体が入れ替わる。サイズも
// 種別も規定内のまま中身だけ変えられるので、検査では止まらない。
func TestAccept_NeverWritesAPublishedKeyTwice(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(okImage(), staging)

	published, err := Accept(context.Background(), store, staging, Media, "42")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// 同じ署名付きURLで別の中身を置き直す（検査は通る大きさ・種別のまま）。
	swapped := okImage()
	swapped.ETag = "etag-swapped"
	store.objects[staging] = swapped

	republished, err := Accept(context.Background(), store, staging, Media, "42")
	if err != nil {
		t.Fatalf("2回目の Accept: %v", err)
	}

	if republished == published {
		t.Fatalf("2回目が同じキー %q へ写した（公開済みの実体を書き換えられる）", published)
	}
	if store.writes[published] != 1 {
		t.Fatalf("公開済みのキーが %d 回書かれた", store.writes[published])
	}
	if store.objects[published].ETag != "etag-1" {
		t.Fatalf("公開済みの実体が差し替わった: %+v", store.objects[published])
	}
}

// TestAccept_ConcurrentAcceptsNeverShareADestination は、「写す前に写し先が空か
// 見る」では塞がらない順序を再現する。
//
// 2つの受け入れがどちらも「写し先は空」を見たあと、それぞれ別の中身を測って
// 同じ写し先へ順に書ける。見てから書くまでの間が空く以上、事前の確認では直らない。
func TestAccept_ConcurrentAcceptsNeverShareADestination(t *testing.T) {
	const n = 8
	staging := mediaKey(t)
	store := newFakeStore(okImage(), staging)

	keys := make([]string, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			keys[i], errs[i] = Accept(context.Background(), store, staging, Media, "42")
		}(i)
	}
	close(start)
	wg.Wait()

	seen := map[string]bool{}
	for i, key := range keys {
		if errs[i] != nil {
			// 先に受け入れた側が staging を消せば、後続は「置かれていない」。
			// それは正しい答えなので、失敗そのものは咎めない。
			continue
		}
		if seen[key] {
			t.Fatalf("2つの受け入れが同じ写し先 %q を使った", key)
		}
		seen[key] = true
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for key, count := range store.writes {
		if count != 1 {
			t.Fatalf("キー %q が %d 回書かれた", key, count)
		}
	}
}

// 実物を測れないときは写さないこと。測れていないものを公開すると、上限も種別も
// 効いていない実体が公開URLから配られる。
func TestAccept_DoesNotPublishWhatItCannotMeasure(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(okImage(), staging)
	store.statErr = context.DeadlineExceeded

	if _, err := Accept(context.Background(), store, staging, Media, "42"); err == nil {
		t.Fatal("測れないまま受け入れてしまった")
	}
	if len(store.copied) != 0 {
		t.Fatalf("copied = %v, want 写さない", store.copied)
	}
}

// 測ったときの中身と同じものを写す条件が付いていること。
//
// 測る（StatObject）と写す（CopyObject）の間にもう一度置かれると、検査していない
// ものが公開される。この隙間は、署名付きURLを握っている側が好きなときに狙える。
func TestAccept_CopiesOnlyWhatItMeasured(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(okImage(), staging)

	if _, err := Accept(context.Background(), store, staging, Media, "42"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if len(store.copied) != 1 {
		t.Fatalf("copied = %v", store.copied)
	}
	if store.copied[0].etag != "etag-1" {
		t.Fatalf("コピーに ETag の条件が付いていない: %+v", store.copied[0])
	}
}

// 条件を付けられない（ETag が取れない）なら、写さないこと。
// 無条件に写すと、写す瞬間の中身を保証できない。
func TestAccept_RefusesToPublishWithoutAnETag(t *testing.T) {
	staging := mediaKey(t)
	info := okImage()
	info.ETag = ""
	store := newFakeStore(info, staging)

	if _, err := Accept(context.Background(), store, staging, Media, "42"); err == nil {
		t.Fatal("ETag が無いのに受け入れてしまった")
	}
	if len(store.copied) != 0 {
		t.Fatalf("copied = %v, want 写さない", store.copied)
	}
}

// 写す直前に置き直されてコピーが条件で弾かれたら、受け入れも失敗すること。
func TestAccept_FailsWhenTheSourceChangedBeforeTheCopy(t *testing.T) {
	staging := mediaKey(t)
	store := newFakeStore(okImage(), staging)
	store.copyErr = repository.ErrObjectChanged

	if _, err := Accept(context.Background(), store, staging, Media, "42"); err == nil {
		t.Fatal("写す直前に差し替えられたのに受け入れてしまった")
	}
}
