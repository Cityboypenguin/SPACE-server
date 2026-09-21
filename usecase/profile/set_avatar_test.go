package profile

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

// promotingUploads は本物と同じ性質の Acceptor。
// 受け入れるたびに別のキーへ「公開」し、取り消されたキーを覚える。
type promotingUploads struct {
	n         int
	published []string
	discarded []string
}

func (u *promotingUploads) Accept(_ context.Context, _ uploadusecase.Kind, objectKey string) (string, error) {
	u.n++
	key := "avatars/42/accepted-" + string(rune('a'+u.n-1)) + ".png"
	u.published = append(u.published, key)
	return key, nil
}

func (u *promotingUploads) Discard(_ context.Context, objectKey string) {
	u.discarded = append(u.discarded, objectKey)
}

type failingTx struct{ err error }

func (t failingTx) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if t.err != nil {
		return t.err
	}
	return fn(ctx)
}

type stubProfileRepo struct{ repository.ProfileRepository }

func (stubProfileRepo) SetAvatarMedia(context.Context, int64, int64) error { return nil }
func (stubProfileRepo) GetProfileByUserID(context.Context, int64) (*model.Profile, error) {
	return &model.Profile{}, nil
}

type stubMediaWriter struct{ repository.MediaWriter }

func (stubMediaWriter) CreateMedia(_ context.Context, m *model.Media) error {
	m.ID = 1
	return nil
}

func userCtx(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID, Role: "user"})
}

// TestSetAvatar_DiscardsThePublishedObjectWhenTheSaveFails は今回の本体。
//
// 受け入れは実体を新しいキーへ写す。そのあとのDB保存が失敗すると、どこからも
// 参照されない実体がそのキーに残り続ける。以前は最終キーが staging の名前から
// 導けたので、やり直した要求が同じキーを拾い直していた。
func TestSetAvatar_DiscardsThePublishedObjectWhenTheSaveFails(t *testing.T) {
	uploads := &promotingUploads{}
	saveErr := errors.New("db is down")
	uc := NewSetAvatarUseCase(uploads, stubProfileRepo{}, stubMediaWriter{}, failingTx{err: saveErr})

	if _, err := uc.Execute(userCtx(42), "staging/avatars/42/x.png"); !errors.Is(err, saveErr) {
		t.Fatalf("error = %v, want %v", err, saveErr)
	}
	if len(uploads.published) != 1 {
		t.Fatalf("published = %v", uploads.published)
	}
	if len(uploads.discarded) != 1 || uploads.discarded[0] != uploads.published[0] {
		t.Fatalf("discarded = %v, want [%q]", uploads.discarded, uploads.published[0])
	}
}

// 保存できたものは消さないこと。
func TestSetAvatar_KeepsThePublishedObjectWhenTheSaveSucceeds(t *testing.T) {
	uploads := &promotingUploads{}
	uc := NewSetAvatarUseCase(uploads, stubProfileRepo{}, stubMediaWriter{}, failingTx{})

	if _, err := uc.Execute(userCtx(42), "staging/avatars/42/x.png"); err != nil {
		t.Fatal(err)
	}
	if len(uploads.discarded) != 0 {
		t.Fatalf("discarded = %v, want 保存できたものは消さない", uploads.discarded)
	}
}

// アバターとして受け入れを通していること（種別の取り違えがないこと）。
func TestSetAvatar_AcceptsTheKeyAsAnAvatar(t *testing.T) {
	var got uploadusecase.Kind
	uploads := recordingKind{&got}
	uc := NewSetAvatarUseCase(uploads, stubProfileRepo{}, stubMediaWriter{}, failingTx{})

	if _, err := uc.Execute(userCtx(42), "avatars/42/x.png"); err != nil {
		t.Fatal(err)
	}
	if got != uploadusecase.Avatar {
		t.Fatalf("受け入れた種別 = %q, want %q", got, uploadusecase.Avatar)
	}
}

type recordingKind struct{ got *uploadusecase.Kind }

func (r recordingKind) Accept(_ context.Context, kind uploadusecase.Kind, objectKey string) (string, error) {
	*r.got = kind
	return objectKey, nil
}

func (recordingKind) Discard(context.Context, string) {}
