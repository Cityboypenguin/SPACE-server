package announcement

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type deleteAnnouncementRepo struct {
	repository.AnnouncementRepository
	findCalled bool
}

func (r *deleteAnnouncementRepo) FindByID(context.Context, int64) (*model.Announcement, error) {
	r.findCalled = true
	return &model.Announcement{}, nil
}

func TestDeleteAnnouncementRequiresAdminInUseCase(t *testing.T) {
	repo := &deleteAnnouncementRepo{}
	uc := NewDeleteAnnouncementUseCase(repo)
	ctx := auth.WithClaims(context.Background(), &auth.Claims{ID: 1, Role: "user"})
	if err := uc.Execute(ctx, 7); err == nil {
		t.Fatal("non-admin must be rejected")
	}
	if repo.findCalled {
		t.Fatal("repository must not be called before authorization")
	}
}
