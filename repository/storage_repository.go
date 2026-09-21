package repository

import (
	"context"
	"errors"
	"io"
	"time"
)

// ObjectInfo is what StatObject reports about an already-stored object.
type ObjectInfo struct {
	Size        int64
	ContentType string
	// ETag は中身が変わると変わる印。測った時点の中身を指すので、
	// 「測ったものと同じものを写す」条件に使う（CopyObject 参照）。
	ETag string
}

type StorageRepository interface {
	// PresignedPutURL generates a presigned PUT URL for direct-to-storage upload.
	//
	// maxBytes は「このURLで置いてよい上限」だが、署名付き PUT では強制できない。
	// S3 互換の presigned PUT にサイズ条件は付けられず（付けられるのは presigned
	// POST policy）、Azure の SAS にもブロック BLOB のサイズを縛る手段が無い。
	// つまりURLを受け取った利用者は、上限を超えるオブジェクトをそのまま置ける。
	//
	// 実際の強制は「置いたオブジェクトをアプリが受け取る時点」で行う。upload.Verify
	// （internal/upload）が StatObject で実物を測り、上限超過・種別違いを弾いて
	// そのオブジェクトを消す。どこにも参照されないオブジェクトが一時的に残るだけなら
	// 害は無く、参照された瞬間には必ず検査を通る、という形にしてある。
	//
	// maxBytes をここに残してあるのは、バケットポリシーや presigned POST policy へ
	// 移した時にそのまま効かせるため。
	//
	// このURLは有効期間のあいだ**何度でも**書ける。1回使えば無効になる、という
	// ものではない。だから検査に通ったオブジェクトをそのまま公開してはいけない
	// （検査後に中身だけ差し替えられる）。受け入れ時に CopyObject で
	// 署名の及ばないキーへ移すこと。internal/upload がその手順を持っている。
	PresignedPutURL(ctx context.Context, objectKey string, contentType string, expires time.Duration, maxBytes int64) (string, error)
	// StatObject returns the stored object's size and content type.
	// 存在しないオブジェクトには ErrObjectNotFound を返す。
	StatObject(ctx context.Context, objectKey string) (ObjectInfo, error)
	// CopyObject copies srcKey to dstKey inside the same bucket/container,
	// server side（中身はこのプロセスを通らない）。
	//
	// 署名付きURLで置かれたオブジェクトを、URLの及ばないキーへ移すために使う。
	// 検査したオブジェクトをそのまま公開すると、URLの有効期間内に中身だけ
	// 差し替えられる（PresignedPutURL のコメント参照）。
	//
	// srcETag は「測ったときの中身」。コピー元がそれと違っていれば
	// ErrObjectChanged を返し、何も書かない。条件を付けるのは、測ってから写す
	// までの隙間にコピー元を差し替えられるため。署名付きURLは有効な間ずっと
	// 書けるので、この隙間は利用者が好きなときに狙える。条件なしで写すと、
	// 「検査したもの」と「公開したもの」が別物になりうる。
	//
	// dstKey は呼び出しのたびに引き直された、まだ誰も知らないキーであること
	// （internal/upload.acceptedKeyFor がそれを保証する）。この契約に
	// 「宛先が既に在れば失敗する」条件は無い。付けても守れないためで、MinIO は
	// CopyObject の If-None-Match: * を黙って無視して上書きする（実機で確認済み）。
	// 守れない条件を契約に書くと、呼び出し側が守られているつもりで書き換えを
	// 許してしまう。上書きが起きない根拠は宛先の一意さに置いてある。
	CopyObject(ctx context.Context, srcKey, dstKey, srcETag string) error
	PublicURL(objectKey string) string
	DeleteObject(ctx context.Context, objectKey string) error
}

// ErrObjectNotFound is returned by StatObject when the object does not exist.
// 「まだアップロードしていないキーを申告された」を、ストレージ障害と区別するために使う。
var ErrObjectNotFound = errors.New("object not found")

// ErrObjectChanged is returned by CopyObject when the source no longer matches
// the ETag the caller measured. 検査と公開の間に中身が差し替えられた合図。
var ErrObjectChanged = errors.New("object changed since it was verified")

type PrivateStorageRepository interface {
	PutPrivateObject(ctx context.Context, objectKey, contentType string, body io.Reader, size int64) error
	OpenPrivateObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
	ListPrivateObjects(ctx context.Context, prefix string) ([]PrivateObject, error)
	DeletePrivateObject(ctx context.Context, objectKey string) error
}

type PrivateObject struct {
	Key          string
	LastModified time.Time
}
