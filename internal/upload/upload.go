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
package upload

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/repository"
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
	Avatar = Kind{Prefix: "avatars", MaxBytes: 5 * mb, Exts: imageExts, Label: "アバター画像"}
	// Media は投稿・メッセージ・質問・回答の添付。
	Media         = Kind{Prefix: "media", MaxBytes: 20 * mb, Exts: imageExts, Label: "添付画像"}
	CommunityIcon = Kind{Prefix: "community-icons", MaxBytes: 5 * mb, Exts: imageExts, Label: "コミュニティアイコン"}
	TermsDocument = Kind{Prefix: "terms", MaxBytes: 5 * mb, Exts: map[string]string{"text/markdown": ".md"}, Label: "規約ドキュメント"}
)

var kinds = []Kind{Avatar, Media, CommunityIcon, TermsDocument}

// Ext は contentType に対応する拡張子を返す。受け付けない種別なら ok が false。
func (k Kind) Ext(contentType string) (string, bool) {
	ext, ok := k.Exts[contentType]
	return ext, ok
}

// KindForObjectKey はオブジェクトキーの先頭セグメントから種別を引く。
//
// 種別が引けないキーは、こちらが出した署名付きURLに対応しない。上限も種別も
// 決まらないので受け入れない（呼び出し側が ok==false を弾く）。
func KindForObjectKey(objectKey string) (Kind, bool) {
	prefix, _, ok := strings.Cut(objectKey, "/")
	if !ok {
		return Kind{}, false
	}
	for _, k := range kinds {
		if k.Prefix == prefix {
			return k, true
		}
	}
	return Kind{}, false
}

// ObjectStore は Verify が要るぶんだけの StorageRepository。
// repository.StorageRepository がそのまま満たす。
type ObjectStore interface {
	StatObject(ctx context.Context, objectKey string) (repository.ObjectInfo, error)
	DeleteObject(ctx context.Context, objectKey string) error
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
	kind, ok := KindForObjectKey(objectKey)
	if !ok {
		return fmt.Errorf("invalid object key")
	}

	info, err := store.StatObject(ctx, objectKey)
	if err != nil {
		if err == repository.ErrObjectNotFound {
			return fmt.Errorf("アップロードされていないファイルが指定されました")
		}
		// ストレージ障害と「上限超過」を言い分けない理由は無いので、ここは素直に
		// 失敗として返す。通ってしまうと検査そのものが素通しになる。
		return fmt.Errorf("failed to verify the uploaded file")
	}

	if info.Size > kind.MaxBytes {
		_ = store.DeleteObject(ctx, objectKey)
		return fmt.Errorf("%sのサイズが上限（%dMB）を超えています", kind.Label, kind.MaxBytes/mb)
	}
	// Content-Type も見る。サイズだけ見ても、画像のつもりの口へ実行可能ファイルを
	// 置かれると、そのまま公開URLから配られる。空は「申告が無い」なので弾く。
	if _, ok := kind.Ext(info.ContentType); !ok {
		_ = store.DeleteObject(ctx, objectKey)
		return fmt.Errorf("%sの形式が受け付けられません: %s", kind.Label, info.ContentType)
	}
	return nil
}
