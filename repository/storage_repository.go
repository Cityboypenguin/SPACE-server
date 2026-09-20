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
	PresignedPutURL(ctx context.Context, objectKey string, contentType string, expires time.Duration, maxBytes int64) (string, error)
	// StatObject returns the stored object's size and content type.
	// 存在しないオブジェクトには ErrObjectNotFound を返す。
	StatObject(ctx context.Context, objectKey string) (ObjectInfo, error)
	PublicURL(objectKey string) string
	DeleteObject(ctx context.Context, objectKey string) error
}

// ErrObjectNotFound is returned by StatObject when the object does not exist.
// 「まだアップロードしていないキーを申告された」を、ストレージ障害と区別するために使う。
var ErrObjectNotFound = errors.New("object not found")

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
