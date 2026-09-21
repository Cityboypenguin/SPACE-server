package question

import (
	"context"
	"errors"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

// promotingUploads は本物と同じ性質の Acceptor。
// 受け入れるたびに別のキーへ「公開」し、取り消されたキーを覚える。
type promotingUploads struct {
	n         int
	kinds     []uploadusecase.Kind
	published []string
	discarded []string
}

func (u *promotingUploads) Accept(_ context.Context, kind uploadusecase.Kind, _ string) (string, error) {
	u.n++
	u.kinds = append(u.kinds, kind)
	key := "media/7/accepted-" + string(rune('a'+u.n-1)) + ".png"
	u.published = append(u.published, key)
	return key, nil
}

func (u *promotingUploads) Discard(_ context.Context, objectKey string) {
	u.discarded = append(u.discarded, objectKey)
}

type fakeQuestionRepoForCreate struct {
	repository.QuestionRepository
	err error
}

func (f *fakeQuestionRepoForCreate) SaveQuestion(_ context.Context, q *model.Question) error {
	if f.err != nil {
		return f.err
	}
	q.ID = 1
	return nil
}

type fakeMediaRepoForCreate struct{ mediaAttachRepository }

func (fakeMediaRepoForCreate) CreateMediaBatch(_ context.Context, medias []*model.Media) error {
	for i, m := range medias {
		m.ID = int64(i + 1)
	}
	return nil
}

func (fakeMediaRepoForCreate) CreateQuestionMediaBatch(context.Context, int64, []int64, int) error {
	return nil
}

type fakeAnonIdentity struct{}

func (fakeAnonIdentity) Execute(context.Context, int64, int64) (*model.RoomAnonymousIdentity, error) {
	return &model.RoomAnonymousIdentity{}, nil
}

func newCreateQuestionFixture(saveErr error) (*promotingUploads, CreateQuestionUseCase) {
	uploads := &promotingUploads{}
	uc := NewCreateQuestionUseCase(nil, uploads,
		&fakeQuestionRepoForCreate{err: saveErr},
		fakeMediaRepoForCreate{},
		fakeTxManager{}, &fakeRequireWritable{}, fakeAnonIdentity{})
	return uploads, uc
}

func stagingInputs() []model.MediaInput {
	return []model.MediaInput{
		{StorageKey: "staging/media/7/a.png", ContentType: "image/png"},
		{StorageKey: "staging/media/7/b.png", ContentType: "image/png"},
	}
}

// 保存に失敗したら、受け入れで公開した実体をすべて取り消すこと。
// 取り消さないと、どこからも参照されない実体がそのまま残り続ける。
func TestCreateQuestion_DiscardsPublishedAttachmentsWhenTheSaveFails(t *testing.T) {
	saveErr := errors.New("db is down")
	uploads, uc := newCreateQuestionFixture(saveErr)

	if _, err := uc.Execute(askerCtx(7), 5, "本文", stagingInputs()); !errors.Is(err, saveErr) {
		t.Fatalf("error = %v, want %v", err, saveErr)
	}
	if len(uploads.published) != 2 {
		t.Fatalf("published = %v", uploads.published)
	}
	if len(uploads.discarded) != 2 {
		t.Fatalf("discarded = %v, want 公開した2件とも取り消すこと", uploads.discarded)
	}
}

// TestCreateQuestion_KeepsAttachmentsWhenTheSaveSucceeds は後始末の裏側。
//
// 「成功したら消さない」を確かめないと、後始末の合図を取り違えたときに
// **成功した投稿の添付を消す**という、はるかに悪い壊れ方をしても気づけない。
func TestCreateQuestion_KeepsAttachmentsWhenTheSaveSucceeds(t *testing.T) {
	uploads, uc := newCreateQuestionFixture(nil)

	q, err := uc.Execute(askerCtx(7), 5, "本文", stagingInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q == nil {
		t.Fatal("質問が返らなかった")
	}
	if len(uploads.discarded) != 0 {
		t.Fatalf("discarded = %v, want 保存できた添付は消さない", uploads.discarded)
	}
}

// 添付として受け入れを通していること（種別の取り違えがないこと）。
func TestCreateQuestion_AcceptsKeysAsAttachments(t *testing.T) {
	uploads, uc := newCreateQuestionFixture(nil)

	if _, err := uc.Execute(askerCtx(7), 5, "本文", stagingInputs()); err != nil {
		t.Fatal(err)
	}
	for _, kind := range uploads.kinds {
		if kind != uploadusecase.Attachment {
			t.Fatalf("受け入れた種別 = %q, want %q", kind, uploadusecase.Attachment)
		}
	}
}
