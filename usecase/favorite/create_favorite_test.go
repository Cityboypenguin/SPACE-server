package favorite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	notificationuc "github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// Minimal interface-embedding fakes: only the methods CreateFavoriteInteractor
// calls are overridden; anything else panics if unexpectedly invoked.
type fakePostRepo struct {
	repository.PostRepository
	post *model.Post
}

func (f *fakePostRepo) GetPostByID(_ context.Context, _ int64) (*model.Post, error) {
	return f.post, nil
}

type fakeFavoriteRepo struct {
	repository.FavoriteRepository
	existing *model.Favorite
	created  *model.Favorite
}

func (f *fakeFavoriteRepo) GetFavoriteByUserIDAndPostID(_ context.Context, _ int64, _ int64) (*model.Favorite, error) {
	if f.existing == nil {
		return nil, sql.ErrNoRows
	}
	return f.existing, nil
}

func (f *fakeFavoriteRepo) CreateFavorite(_ context.Context, fav *model.Favorite) (int64, error) {
	f.created = fav
	return 99, nil
}

type recordingPublisher struct {
	calls []notificationuc.PublishParams
}

func (r *recordingPublisher) Publish(_ context.Context, p notificationuc.PublishParams) error {
	r.calls = append(r.calls, p)
	return nil
}

func (r *recordingPublisher) PublishBatch(_ context.Context, _ []notificationuc.PublishParams) error {
	return nil
}

func TestCreateFavorite_RejectsSelfFavorite(t *testing.T) {
	postRepo := &fakePostRepo{post: &model.Post{ID: 42, UserID: 7}}
	pub := &recordingPublisher{}
	uc := NewCreateFavoriteUseCase(&fakeFavoriteRepo{}, postRepo, pub)

	if _, err := uc.Execute(context.Background(), model.CreateFavoriteParam{UserID: 7, PostID: 42}); err == nil {
		t.Fatal("expected error favoriting one's own post")
	}
	if len(pub.calls) != 0 {
		t.Fatal("no notification should be published on rejected self-favorite")
	}
}

func TestCreateFavorite_RejectsDuplicate(t *testing.T) {
	postRepo := &fakePostRepo{post: &model.Post{ID: 42, UserID: 5}}
	favRepo := &fakeFavoriteRepo{existing: &model.Favorite{ID: 1, UserID: 7, PostID: 42}}
	uc := NewCreateFavoriteUseCase(favRepo, postRepo, &recordingPublisher{})

	if _, err := uc.Execute(context.Background(), model.CreateFavoriteParam{UserID: 7, PostID: 42}); err == nil {
		t.Fatal("expected error on duplicate favorite")
	}
}

func TestCreateFavorite_NotifiesPostOwner(t *testing.T) {
	postRepo := &fakePostRepo{post: &model.Post{ID: 42, UserID: 5}}
	favRepo := &fakeFavoriteRepo{}
	pub := &recordingPublisher{}
	uc := NewCreateFavoriteUseCase(favRepo, postRepo, pub)

	fav, err := uc.Execute(context.Background(), model.CreateFavoriteParam{UserID: 7, PostID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fav.ID != 99 {
		t.Errorf("favorite ID = %d, want 99", fav.ID)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected exactly one notification, got %d", len(pub.calls))
	}
	got := pub.calls[0]
	if got.UserID != 5 {
		t.Errorf("notification recipient = %d, want post owner 5", got.UserID)
	}
	if got.Type != notificationuc.TypeFavorite {
		t.Errorf("notification type = %q, want %q", got.Type, notificationuc.TypeFavorite)
	}
	if got.ActorID == nil || *got.ActorID != 7 {
		t.Errorf("notification actor = %v, want 7", got.ActorID)
	}
}

func TestCreateFavorite_SucceedsWithoutPublisher(t *testing.T) {
	postRepo := &fakePostRepo{post: &model.Post{ID: 42, UserID: 5}}
	uc := NewCreateFavoriteUseCase(&fakeFavoriteRepo{}, postRepo, nil)

	if _, err := uc.Execute(context.Background(), model.CreateFavoriteParam{UserID: 7, PostID: 42}); err != nil {
		t.Fatalf("nil publisher must be tolerated, got error: %v", err)
	}
}
