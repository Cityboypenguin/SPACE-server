package upload

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

// ctxAs は指定した利用者として呼び出す context を作る。
// 所有者付きの種別は、キーの所有者と呼び出し元が一致しないと受け入れられない。
func ctxAs(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID})
}

const testOwner = int64(42)

var okInfo = repository.ObjectInfo{Size: 10, ContentType: "image/png", ETag: "etag-1"}

// stagedIn は種別ぶんの staging キーを1つ用意し、その置き場に実体を置く。
func stagedIn(t *testing.T, kind Kind) (string, *fakeStore) {
	t.Helper()
	key, err := kind.NewStagingKey("42", "image/png")
	if err != nil {
		t.Fatalf("NewStagingKey: %v", err)
	}
	return key, newFakeStore(okInfo, key)
}

// 公開の種別は公開側へ、非公開の種別は非公開側へ入ること。
// 取り違えると DM の添付が匿名読み取りを許した置き場に入る。
func TestAcceptor_RoutesEachKindToItsOwnStore(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		ucKind  uploadusecase.Kind
		private bool
	}{
		{"投稿の添付は公開側", Media, uploadusecase.Attachment, false},
		{"DMの添付は非公開側", MessageMedia, uploadusecase.MessageAttachment, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, store := stagedIn(t, tt.kind)
			public, private := newFakeStore(okInfo), newFakeStore(okInfo)
			if tt.private {
				private = store
			} else {
				public = store
			}

			a := NewAcceptor(public, private)
			accepted, err := a.Accept(ctxAs(testOwner), tt.ucKind, key)
			if err != nil {
				t.Fatalf("Accept: %v", err)
			}

			used, unused := public, private
			if tt.private {
				used, unused = private, public
			}
			if len(used.copied) != 1 {
				t.Fatalf("使うべき置き場への複製が %d 件（1件のはず）", len(used.copied))
			}
			if used.copied[0].dst != accepted {
				t.Errorf("複製先 %q, want %q", used.copied[0].dst, accepted)
			}
			if len(unused.copied) != 0 {
				t.Errorf("使わない置き場に %d 件書かれた", len(unused.copied))
			}
		})
	}
}

// 非公開の置き場が配られていないとき、公開側へ落とさずに失敗すること。
// 黙って公開側へ倒すと、配線漏れが「DM の添付が公開される」形で出る。
func TestAcceptor_RefusesPrivateKindsWithoutAPrivateStore(t *testing.T) {
	key, _ := stagedIn(t, MessageMedia)
	public := newFakeStore(okInfo, key)

	a := NewAcceptor(public, nil)
	if _, err := a.Accept(ctxAs(testOwner), uploadusecase.MessageAttachment, key); err == nil {
		t.Fatal("非公開の置き場が無いのに受け入れられた")
	}
	if len(public.copied) != 0 {
		t.Errorf("公開側に %d 件書かれた（1件も書いてはいけない）", len(public.copied))
	}
}

// Discard もキーから置き場を引くこと。引けないと取り消しが空振りし、
// 参照されないオブジェクトが残る。
func TestAcceptor_DiscardUsesTheStoreThatHoldsTheKey(t *testing.T) {
	publicKey, public := stagedIn(t, Media)
	privateKey, private := stagedIn(t, MessageMedia)

	a := NewAcceptor(public, private)
	a.Discard(context.Background(), publicKey)
	a.Discard(context.Background(), privateKey)

	if len(public.deleted) != 1 || public.deleted[0] != publicKey {
		t.Errorf("公開側の削除 = %v, want [%s]", public.deleted, publicKey)
	}
	if len(private.deleted) != 1 || private.deleted[0] != privateKey {
		t.Errorf("非公開側の削除 = %v, want [%s]", private.deleted, privateKey)
	}
}

// こちらが払い出した形でないキーは、どちらの置き場にも触らないこと。
func TestAcceptor_DiscardIgnoresForeignKeys(t *testing.T) {
	public, private := newFakeStore(okInfo), newFakeStore(okInfo)
	a := NewAcceptor(public, private)

	a.Discard(context.Background(), "../../etc/passwd")
	a.Discard(context.Background(), "unknown-prefix/1/x.png")

	if len(public.deleted)+len(private.deleted) != 0 {
		t.Errorf("削除が起きた: public=%v private=%v", public.deleted, private.deleted)
	}
}
