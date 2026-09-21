package upload

import (
	"context"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

// Acceptor は ObjectStore を束ねた Accept の入口。
//
// ユースケース層は usecase/upload.Acceptor として受け取る。これはその実装で、
// 配線（cmd/server/main.go）が1つ作って配る。
//
// 「誰のキーか」はここで ctx から決める。ユースケースに引数で言わせると、
// 正しい値を渡す責任がそちらに残り、入口が増えたときにそこだけ取り違えられる
// （internal/authz.CallerID のコメントと同じ理由）。
type Acceptor struct {
	store ObjectStore
}

var _ uploadusecase.Acceptor = (*Acceptor)(nil)

func NewAcceptor(store ObjectStore) *Acceptor {
	return &Acceptor{store: store}
}

// Accept は申告されたキーを受け入れ、保存してよいキーを返す。
// 空文字は「指定なし」としてそのまま返す（省略可能なキーの経路があるため）。
func (a *Acceptor) Accept(ctx context.Context, kind uploadusecase.Kind, objectKey string) (string, error) {
	if objectKey == "" {
		return "", nil
	}
	want, ok := KindForPrefix(string(kind))
	if !ok {
		return "", fmt.Errorf("unknown upload kind: %s", kind)
	}

	var owner string
	if want.Owned {
		callerID, err := authz.CallerID(ctx)
		if err != nil {
			return "", err
		}
		owner = fmt.Sprintf("%d", callerID)
	}
	return Accept(ctx, a.store, objectKey, want, owner)
}

// Discard は受け入れで公開したオブジェクトを取り消す（保存に失敗したとき）。
func (a *Acceptor) Discard(ctx context.Context, objectKey string) {
	Discard(ctx, a.store, objectKey)
}
