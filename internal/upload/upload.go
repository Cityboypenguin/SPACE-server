// Package upload は署名付きアップロードの「上限」と「受け付ける種別」を
// 1か所で決め、その通りになっているかを受け入れ時に確かめる。
//
// 署名付き PUT では上限を強制できない。S3 互換の presigned PUT にサイズ条件は
// 付けられず（付けられるのは presigned POST policy）、Azure の SAS にも
// ブロック BLOB のサイズを縛る手段が無い。URL を1本受け取った利用者は、
// 20MB と言われたところへ何GBでも置ける。
//
// そこで強制は「置いたオブジェクトをアプリが受け取る時点」に置く。アバターの
// 設定・添付の登録・規約の差し替えは、いずれもオブジェクトキーをサーバーへ
// 渡してはじめて意味を持つので、その入口で実物を測れば「参照されているのに
// 上限を超えているオブジェクト」は存在しなくなる。どこからも参照されないまま
// 転がっているオブジェクトは、課金以外の害が無い。
//
// 上限を Kind の表に集めてあるのは、URL を出す側と受け入れる側で別々に数字を
// 書くと必ず食い違うため。片方だけ 20MB に上げると、上げたつもりの添付が
// 受け入れ時に弾かれる（あるいは逆に、絞ったつもりの上限が素通しになる）。
//
// # 検査しただけでは足りない
//
// 署名付きURLは有効期間のあいだ何度でも書ける。検査に通ったオブジェクトを
// そのまま公開すると、
//
//  1. 規定内の画像を置く
//  2. サーバーが測って受け入れ、キーを DB に保存する
//  3. **同じURLで中身だけ差し替える**（URLはまだ15分有効）
//
// で、「参照されているものは検査済み」という前提が崩れる。サイズも Content-Type も
// 検査後の値とは無関係になり、画像のつもりの公開URLから任意のバイト列が配られる。
//
// そこで、署名付きURLが書ける場所（staging/…）と、アプリが参照する場所を分ける。
// 受け入れ時に staging から最終キーへ写し、staging のオブジェクトを消す。
// 最終キーに対する署名付きURLは一度も発行していないので、写した後の実体を
// 書き換える手段が無い。写すのは保管側で完結する（中身はこのプロセスを通らない）。
//
// 最終キーは受け入れ1回につき1つ、その場で引き直す（acceptedKeyFor）。staging の
// 名前から導くと、同じ staging キーを2回申告されたときに写し先がぶつかる。
package upload

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/google/uuid"
)

// Kind は署名付きURLを出す単位。オブジェクトキーの先頭セグメントで引ける。
type Kind struct {
	// Prefix はオブジェクトキーの先頭セグメント（"avatars/123/x.jpg" なら "avatars"）。
	Prefix string
	// MaxBytes はこの種別で受け入れる最大サイズ。
	MaxBytes int64
	// Exts は受け付ける Content-Type と、キーに付ける拡張子。
	Exts map[string]string
	// Label はエラー文言に出す名前。
	Label string
	// Owned は「キーに所有者セグメントが要る種別か」。
	//
	// 要る種別では、申告されたキーの所有者が呼び出し元本人でなければ受け入れない。
	// 他人のキーを申告できると、まだ受け入れていない他人のアップロードを
	// こちらの操作で消したり写したりできてしまう（Accept のコメント参照）。
	Owned bool
}

const (
	mb = 1024 * 1024
)

// imageExts は画像アップロード3種で共通。別々に持つと片方だけ種別が増える。
var imageExts = map[string]string{
	"image/jpeg":    ".jpg",
	"image/png":     ".png",
	"image/webp":    ".webp",
	"image/gif":     ".gif",
	"image/svg+xml": ".svg",
}

var (
	Avatar = Kind{Prefix: "avatars", MaxBytes: 5 * mb, Exts: imageExts, Label: "アバター画像", Owned: true}
	// Media は投稿・メッセージ・質問・回答の添付。
	Media         = Kind{Prefix: "media", MaxBytes: 20 * mb, Exts: imageExts, Label: "添付画像", Owned: true}
	CommunityIcon = Kind{Prefix: "community-icons", MaxBytes: 5 * mb, Exts: imageExts, Label: "コミュニティアイコン", Owned: true}
	// 規約ドキュメントは管理者だけが置く。誰のものでもないので所有者セグメントは無い。
	TermsDocument = Kind{Prefix: "terms", MaxBytes: 5 * mb, Exts: map[string]string{"text/markdown": ".md"}, Label: "規約ドキュメント"}
)

var kinds = []Kind{Avatar, Media, CommunityIcon, TermsDocument}

// StagingPrefix は署名付きURLが書き込める唯一の場所。
//
// ここ以外のキーに対して署名付きURLを出してはいけない。出した時点で、
// そのキーの中身は有効期間のあいだ差し替え可能になる（パッケージコメント参照）。
const StagingPrefix = "staging"

// Ext は contentType に対応する拡張子を返す。受け付けない種別なら ok が false。
func (k Kind) Ext(contentType string) (string, bool) {
	ext, ok := k.Exts[contentType]
	return ext, ok
}

// NewStagingKey は1回のアップロードぶんのキーを作る。返すのは staging 側
// （署名付きURLを出す先）で、最終キーは受け入れ時にここから導く。
//
// キーを組み立てる場所と、受け入れ時にキーを検める場所（parseKey）を同じ
// パッケージに置いてある。別々に書くと、生成の形を変えたときに検める側が
// 置いていかれ、「出したばかりのキーが受け入れられない」あるいは
// 「想定しない形のキーが通る」のどちらかになる。
//
// ownerSegment は利用者ごとに分けるための2段目。空なら省く。
func (k Kind) NewStagingKey(ownerSegment, contentType string) (string, error) {
	ext, ok := k.Ext(contentType)
	if !ok {
		return "", fmt.Errorf("unsupported content type: %s", contentType)
	}
	name := uuid.NewString() + ext
	if ownerSegment == "" {
		return StagingPrefix + "/" + k.Prefix + "/" + name, nil
	}
	return StagingPrefix + "/" + k.Prefix + "/" + ownerSegment + "/" + name, nil
}

// KindForObjectKey はオブジェクトキーから種別を引く。staging のキーでも引ける。
//
// 種別が引けないキーは、こちらが出した署名付きURLに対応しない。上限も種別も
// 決まらないので受け入れない（呼び出し側が ok==false を弾く）。
func KindForObjectKey(objectKey string) (Kind, bool) {
	parsed, err := parseKey(objectKey)
	return parsed.kind, err == nil
}

// KindForPrefix はキーの先頭セグメントから種別を引く。
// ユースケース側が「受け付ける種別」を名前で指定するための口（usecase/upload.Kind）。
func KindForPrefix(prefix string) (Kind, bool) {
	for _, k := range kinds {
		if k.Prefix == prefix {
			return k, true
		}
	}
	return Kind{}, false
}

// parsedKey は申告されたキーを読み解いた結果。
type parsedKey struct {
	kind    Kind
	staging bool
	// owner は所有者セグメント。所有者を持たない種別では空。
	owner string
}

// parseKey は申告されたキーが「こちらが出した形か」を確かめ、種別と
// staging かどうかを返す。
//
// 形まで見るのは、キーが利用者から送られてくるため。先頭セグメントだけ見て
// 通すと、avatars/ で始まりさえすれば任意のキーを申告できる。相対参照（..）を
// 混ぜたキーは公開URLを組み立てた先でバケットの外を指しうるし、他人のキーを
// そのまま名乗ることもできる。UUIDの形まで要求しておけば、通るのは
// 「こちらが払い出したキー」だけになる。
func parseKey(objectKey string) (parsedKey, error) {
	rest, staging := strings.CutPrefix(objectKey, StagingPrefix+"/")
	segments := strings.Split(rest, "/")
	// <種別>/<UUID><拡張子> か <種別>/<所有者>/<UUID><拡張子> のどちらか。
	if len(segments) != 2 && len(segments) != 3 {
		return parsedKey{}, errInvalidKey
	}

	kind, found := KindForPrefix(segments[0])
	if !found {
		return parsedKey{}, errInvalidKey
	}

	var owner string
	if len(segments) == 3 {
		if !isDecimal(segments[1]) {
			return parsedKey{}, errInvalidKey
		}
		owner = segments[1]
	}

	name := segments[len(segments)-1]
	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return parsedKey{}, errInvalidKey
	}
	if !kind.hasExt(name[dot:]) {
		return parsedKey{}, errInvalidKey
	}
	if _, err := uuid.Parse(name[:dot]); err != nil {
		return parsedKey{}, errInvalidKey
	}
	return parsedKey{kind: kind, staging: staging, owner: owner}, nil
}

// CheckKey は申告されたキーが「この種別の、この利用者のもの」かを確かめる。
// **ストレージには一切触らない**ので、受け入れ（Accept）より前に通せる。
//
// 受け入れの後に確かめてはいけない。受け入れは実体を写して元を消すので、
// 他人の staging キーを申告されると、その操作が最後に拒否されても
// **相手のアップロードは既に消えている**。順序だけの問題で、検査の内容は同じ。
//
// owner は呼び出し元の利用者ID（文字列）。所有者を持たない種別（規約ドキュメント）
// では見ない。
func CheckKey(objectKey string, want Kind, owner string) error {
	parsed, err := parseKey(objectKey)
	if err != nil {
		return err
	}
	if parsed.kind.Prefix != want.Prefix {
		// 種別まで見るのは、例えば自分のアバターのキーを投稿の添付として
		// 申告する経路を塞ぐため。上限も用途も種別ごとに違う。
		return errInvalidKey
	}
	if want.Owned && (owner == "" || parsed.owner != owner) {
		return errInvalidKey
	}
	return nil
}

var errInvalidKey = errors.New("invalid object key")

func (k Kind) hasExt(ext string) bool {
	for _, e := range k.Exts {
		if e == ext {
			return true
		}
	}
	return false
}

func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// acceptedKeyFor は「この受け入れぶん」の最終キーを作る。種別と所有者の区切りは
// staging のキーから引き継ぎ、名前（UUID）だけをその場で引き直す。
//
// staging の名前をそのまま使わないのは、それだと写し先が staging キーから導けて
// しまうため。署名付きURLは有効な間ずっと書けるので、同じ staging キーに対する
// 受け入れは何度でも起こせる。写し先が同じなら、あとから来た受け入れが
// **公開済みの実体を上書きする**（Accept のコメント参照）。
//
// 毎回引き直せば、写し先は「今この呼び出しだけが知っているキー」になる。そこへ
// 書くのはこの1回の CopyObject だけで、署名付きURLも出していない。つまり、
// 公開されたオブジェクトの中身は保管側の条件付き書き込みに頼らずに固定される。
func acceptedKeyFor(stagingKey string) (string, error) {
	rest, ok := strings.CutPrefix(stagingKey, StagingPrefix+"/")
	if !ok {
		return "", errInvalidKey
	}
	slash := strings.LastIndex(rest, "/")
	dot := strings.LastIndex(rest, ".")
	if slash < 0 || dot < slash {
		return "", errInvalidKey
	}
	return rest[:slash+1] + uuid.NewString() + rest[dot:], nil
}

// ObjectStore は Accept が要るぶんだけの StorageRepository。
// repository.StorageRepository がそのまま満たす。
type ObjectStore interface {
	StatObject(ctx context.Context, objectKey string) (repository.ObjectInfo, error)
	CopyObject(ctx context.Context, srcKey, dstKey, srcETag string) error
	DeleteObject(ctx context.Context, objectKey string) error
}

// Accept は利用者が申告したキーを受け入れ、アプリが参照してよいキーを返す。
//
// staging のキー（＝いま置かれたばかりのもの）なら、実物を測って検査し、
// 通れば最終キーへ写して staging を消す。返すのはその最終キーで、そこへは
// 署名付きURLを一度も出していない。
//
// staging でないキーは「以前の受け入れで既に写し終えたもの」。編集で添付を
// そのまま送り返す経路があるので素通しするが、存在と種別・上限は確かめる
// （でたらめなキーをそのまま保存しないため）。写す必要は無い。
//
// # 誰のキーかを先に見る
//
// 種別と所有者の確認（CheckKey）は、ストレージに触る前に済ませる。後にすると、
// 他人の staging キーを申告した利用者の操作が最後に拒否されても、そのときには
// 既に実体を写して元を消している。つまり**他人の未確定のアップロードを壊せる**。
// 確認の内容は同じで、順序だけの問題。
//
// # 公開したオブジェクトは二度と書き換わらない
//
// 「写す」だけでは実体は固まらない。署名付きURLは有効な間ずっと書けるので、
// 同じ staging キーに対する受け入れは何度でも起こせる。写し先が staging の名前から
// 導ける作りだと、
//
//  1. 規定内の画像を置く → 受け入れさせる（最終キーへ写る）
//  2. **同じURLで staging に別の中身を置く**
//  3. 同じキーをもう一度申告する → 同じ最終キーへ写し直される
//
// で、公開済みの実体を後から入れ替えられる。サイズも種別も規定内のまま中身だけ
// 変えられるので、検査では止まらない（通報済みの添付を差し替える、といった形で効く）。
//
// 「写す前に写し先が空かどうか見る」では塞がらない。2つの受け入れがどちらも
// 「空」を見たあと、それぞれ別の中身を同じ写し先へ順に書けるため（下の順序）。
//
//	受け入れA: 写し先は空	受け入れB: 写し先は空
//	A に置く → A を測る → A を写す
//	                      B に置く → B を測る → B を写す（A を上書き）
//
// 見てから書くまでの間が空く以上、原子的な「無ければ作る」でなければ直らない。
// そして保管側の条件付き書き込みには頼れない。MinIO は CopyObject の
// If-None-Match: * を黙って無視して上書きする（実機で確認済み）ので、条件を
// 付けたつもりで素通しになる。
//
// そこで写し先そのものを毎回引き直す（acceptedKeyFor）。写し先は呼び出しごとに
// 別のキーになり、ぶつかりようがない。上の順序でも A と B は別のキーへ入り、
// それぞれの要求はそれぞれが検査したものを指す。**どの最終キーも、書かれるのは
// 一度きり**になる。
//
// # 測ってから写すまでの隙間
//
// 測る（StatObject）と写す（CopyObject）の間にもう一度置かれると、検査したものと
// 別のものが公開される。写すときに「測ったときの ETag と同じなら」という条件を
// 付けて塞いである（一致しなければ何も書かれず ErrObjectChanged が返る）。
func Accept(ctx context.Context, store ObjectStore, objectKey string, want Kind, owner string) (string, error) {
	if err := CheckKey(objectKey, want, owner); err != nil {
		return "", fmt.Errorf("invalid object key")
	}
	parsed, err := parseKey(objectKey)
	if err != nil {
		return "", fmt.Errorf("invalid object key")
	}
	if !parsed.staging {
		if _, err := verifyObject(ctx, store, objectKey); err != nil {
			return "", err
		}
		return objectKey, nil
	}

	info, err := verifyObject(ctx, store, objectKey)
	if err != nil {
		return "", err
	}
	if info.ETag == "" {
		// 条件を付けられない＝写す瞬間の中身を保証できない。
		return "", fmt.Errorf("failed to verify the uploaded file")
	}

	finalKey, err := acceptedKeyFor(objectKey)
	if err != nil {
		return "", fmt.Errorf("invalid object key")
	}

	if err := store.CopyObject(ctx, objectKey, finalKey, info.ETag); err != nil {
		if errors.Is(err, repository.ErrObjectChanged) {
			// 測ってから写すまでの間に置き直された。検査していないものは公開しない。
			return "", errNotUploaded
		}
		return "", fmt.Errorf("failed to store the uploaded file")
	}
	// 写した後の staging は用済み。消せなくても受け入れは成立する
	// （最終キーの実体はもう固まっている）ので、失敗は握る。
	_ = store.DeleteObject(ctx, objectKey)
	return finalKey, nil
}

// Discard は受け入れで公開したオブジェクトを取り消す。
//
// 受け入れは1回ごとに新しい最終キーへ写す。その後の保存（DBへの書き込み）が
// 失敗すると、どこからも参照されない実体がそのキーに残り続ける。以前は最終キーが
// staging の名前から導けたので、やり直した要求が同じキーを拾い直していた。
// いまはキーが毎回変わるぶん、取り消しを明示的にやる必要がある。
//
// 消せなくても呼び出し元の失敗は変わらないので、結果は返さない（残るのは
// 参照されないオブジェクトだけで、害は課金に限られる）。ただし黙って捨てると
// 「取り消したつもり」で実体が残るので、失敗はログに残す。掃除の手掛かりは
// そこにしか無い。
func Discard(ctx context.Context, store ObjectStore, objectKey string) {
	if objectKey == "" {
		return
	}
	// 受け入れが作ったキーだけを消す。形の確かめを挟むのは、呼び出し元の
	// 取り違えで無関係なキーを消させないため。
	if _, err := parseKey(objectKey); err != nil {
		return
	}
	if err := store.DeleteObject(ctx, objectKey); err != nil {
		logger.Log.Error().Err(err).
			Str("objectKey", objectKey).
			Msg("failed to discard a published upload; the object is now unreferenced")
	}
}

// Verify は objectKey のオブジェクトが、その種別の上限と Content-Type に
// 収まっていることを確かめる。収まっていなければそのオブジェクトを消してから
// エラーを返す。
//
// 違反したオブジェクトを消すのは、残しておく理由が無いため。受け入れを拒んだ
// 時点でアプリからは二度と参照されず、置いた本人も（キーを握ってはいるが）
// 次の受け入れでまた弾かれる。消し忘れるとストレージにだけ溜まる。
// 消す方が失敗しても受け入れは拒む（エラーはアップロードの可否とは無関係なので握る）。
func Verify(ctx context.Context, store ObjectStore, objectKey string) error {
	_, err := verifyObject(ctx, store, objectKey)
	return err
}

// verifyObject は Verify の本体で、測った結果も返す。
// Accept が「測ったものと同じものを写す」条件（ETag）に使う。
func verifyObject(ctx context.Context, store ObjectStore, objectKey string) (repository.ObjectInfo, error) {
	parsed, err := parseKey(objectKey)
	if err != nil {
		return repository.ObjectInfo{}, fmt.Errorf("invalid object key")
	}
	kind := parsed.kind

	info, err := store.StatObject(ctx, objectKey)
	if err != nil {
		if errors.Is(err, repository.ErrObjectNotFound) {
			return repository.ObjectInfo{}, errNotUploaded
		}
		// ストレージ障害と「上限超過」を言い分けない理由は無いので、ここは素直に
		// 失敗として返す。通ってしまうと検査そのものが素通しになる。
		return repository.ObjectInfo{}, fmt.Errorf("failed to verify the uploaded file")
	}

	if info.Size > kind.MaxBytes {
		_ = store.DeleteObject(ctx, objectKey)
		return repository.ObjectInfo{}, fmt.Errorf("%sのサイズが上限（%dMB）を超えています", kind.Label, kind.MaxBytes/mb)
	}
	// Content-Type も見る。サイズだけ見ても、画像のつもりの口へ実行可能ファイルを
	// 置かれると、そのまま公開URLから配られる。空は「申告が無い」なので弾く。
	if _, ok := kind.Ext(info.ContentType); !ok {
		_ = store.DeleteObject(ctx, objectKey)
		return repository.ObjectInfo{}, fmt.Errorf("%sの形式が受け付けられません: %s", kind.Label, info.ContentType)
	}
	return info, nil
}

// errNotUploaded は「そのキーに実体が無い」。Accept が「写し終えた後の2回目の
// 申告か」を見分けるために、他の失敗と区別している。文言はそのまま利用者へ返る。
var errNotUploaded = errors.New("アップロードされていないファイルが指定されました")
