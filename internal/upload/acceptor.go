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
// 置き場は2つある。種別ごとにどちらへ入れるかが決まっており（Kind.Private）、
// 呼び出し側は選べない。選べる形にすると、DM の添付を公開側へ入れる経路が
// 引数の取り違えだけで生まれる。
type Acceptor struct {
	public  ObjectStore
	private ObjectStore
}

var _ uploadusecase.Acceptor = (*Acceptor)(nil)

// NewAcceptor は公開・非公開の2つの置き場を束ねる。
//
// private が nil なら非公開の種別は受け入れない（黙って公開側へ落とさない）。
// 配線の漏れを、DM の添付が公開されるという形で出さないため。
func NewAcceptor(public, private ObjectStore) *Acceptor {
	return &Acceptor{public: public, private: private}
}

// storeFor は種別に対応する置き場を返す。
func (a *Acceptor) storeFor(k Kind) (ObjectStore, error) {
	if !k.Private {
		return a.public, nil
	}
	if a.private == nil {
		return nil, fmt.Errorf("no private object store is configured for %s", k.Prefix)
	}
	return a.private, nil
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
	store, err := a.storeFor(want)
	if err != nil {
		return "", err
	}
	return Accept(ctx, store, objectKey, want, owner)
}

// Discard は受け入れで公開したオブジェクトを取り消す（保存に失敗したとき）。
//
// どちらの置き場に入れたかはキーの先頭セグメントから引ける（Kind.Private）。
// 引けないキーは受け入れが作ったものではないので、何もしない（Discard 本体も
// 同じ理由で形を検めている）。
func (a *Acceptor) Discard(ctx context.Context, objectKey string) {
	kind, ok := KindForObjectKey(objectKey)
	if !ok {
		return
	}
	store, err := a.storeFor(kind)
	if err != nil {
		return
	}
	Discard(ctx, store, objectKey)
}
