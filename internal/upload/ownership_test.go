package upload

import (
	"context"
	"testing"

	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

// TestAccept_ChecksTheOwnerBeforeTouchingStorage は今回の本体。
//
// 他人の staging キーを申告されたとき、拒むだけでは足りない。受け入れは実体を
// 写して元を消すので、確認が後にあると「操作は拒否されたが相手のアップロードは
// 消えている」になる。相手のミューテーションはそのあと必ず失敗する。
func TestAccept_ChecksTheOwnerBeforeTouchingStorage(t *testing.T) {
	victim, err := Media.NewStagingKey("7", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeStore(okImage(), victim)

	// 利用者42が、利用者7のキーを申告する。
	if _, err := Accept(context.Background(), store, victim, Media, "42"); err == nil {
		t.Fatal("他人のキーを受け入れてしまった")
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want 他人のアップロードに触らないこと", store.deleted)
	}
	if len(store.copied) != 0 {
		t.Fatalf("copied = %v, want 他人のアップロードに触らないこと", store.copied)
	}
	if _, ok := store.objects[victim]; !ok {
		t.Fatal("他人のアップロードが消えた")
	}
}

// 既に公開済みのキー（編集で送り返されたもの）でも、持ち主は見ること。
func TestAccept_ChecksTheOwnerOfAlreadyAcceptedKeys(t *testing.T) {
	victim := acceptedForm(func() string {
		k, err := Media.NewStagingKey("7", "image/png")
		if err != nil {
			t.Fatal(err)
		}
		return k
	}())
	store := newFakeStore(okImage(), victim)

	if _, err := Accept(context.Background(), store, victim, Media, "42"); err == nil {
		t.Fatal("他人の公開済みキーを受け入れてしまった")
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v", store.deleted)
	}
}

// 種別が違うキーは受け入れないこと。
// 自分のアバターのキーを投稿の添付として申告する、といった付け替えを塞ぐ
// （上限も用途も種別ごとに違う）。
func TestAccept_RejectsKeysOfAnotherKind(t *testing.T) {
	own, err := Avatar.NewStagingKey("42", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeStore(okImage(), own)

	if _, err := Accept(context.Background(), store, own, Media, "42"); err == nil {
		t.Fatal("別種別のキーを添付として受け入れてしまった")
	}
	if len(store.copied) != 0 || len(store.deleted) != 0 {
		t.Fatalf("copied = %v deleted = %v, want ストレージに触らないこと", store.copied, store.deleted)
	}
}

// 所有者を持つ種別では、所有者セグメントの無いキーを受け入れないこと。
func TestCheckKey_RequiresAnOwnerSegmentForOwnedKinds(t *testing.T) {
	ownerless, err := Media.NewStagingKey("", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckKey(ownerless, Media, "42"); err == nil {
		t.Fatalf("所有者の無いキー %q を受け入れてしまった", ownerless)
	}
}

// 規約ドキュメントは誰のものでもないので、所有者は見ないこと。
// ここで所有者を要求すると、管理者が規約を差し替えられなくなる。
func TestCheckKey_SkipsOwnershipForUnownedKinds(t *testing.T) {
	key, err := TermsDocument.NewStagingKey("", "text/markdown")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckKey(key, TermsDocument, ""); err != nil {
		t.Fatalf("規約ドキュメントが受け入れられない: %v", err)
	}
}

// TestUsecaseKindsMapToRealKinds は、ユースケース側が名前で指定する種別
// （usecase/upload.Kind）が、こちらの表に必ず対応していることを確かめる。
//
// 対応が切れると、その種別のアップロードは受け入れ時に必ず失敗する。
// URLは出るのに保存できない、という形で出るので気づきにくい。
func TestUsecaseKindsMapToRealKinds(t *testing.T) {
	cases := map[uploadusecase.Kind]Kind{
		uploadusecase.Attachment:    Media,
		uploadusecase.Avatar:        Avatar,
		uploadusecase.CommunityIcon: CommunityIcon,
		uploadusecase.TermsDocument: TermsDocument,
	}
	if len(cases) != len(kinds) {
		t.Fatalf("種別が %d 個あるのに、ユースケース側の名前は %d 個しかない", len(kinds), len(cases))
	}
	for name, want := range cases {
		got, ok := KindForPrefix(string(name))
		if !ok {
			t.Fatalf("種別 %q が引けない", name)
		}
		if got.Prefix != want.Prefix {
			t.Fatalf("種別 %q -> %q, want %q", name, got.Prefix, want.Prefix)
		}
	}
}

// Discard は受け入れで作ったキーを消すこと。保存に失敗したときの後始末。
func TestDiscard_RemovesThePublishedObject(t *testing.T) {
	staging, err := Media.NewStagingKey("42", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeStore(okImage(), staging)

	published, err := Accept(context.Background(), store, staging, Media, "42")
	if err != nil {
		t.Fatal(err)
	}
	Discard(context.Background(), store, published)

	if _, ok := store.objects[published]; ok {
		t.Fatalf("公開した実体 %q が残った", published)
	}
}

// 形の読めないキーでは何も消さないこと。呼び出し元の取り違えで
// 無関係なオブジェクトを消させないため。
func TestDiscard_IgnoresKeysItDidNotIssue(t *testing.T) {
	store := newFakeStore(okImage(), "private/secret.md")
	Discard(context.Background(), store, "private/secret.md")

	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want 何も消さないこと", store.deleted)
	}
}
