package community

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type avatarCommunityRepo struct {
	repository.CommunityRepository
	community     *model.Community
	savedAvatar   *repository.UpdateCommunityAvatarParam
	updatedAvatar *repository.UpdateCommunityAvatarParam
	updateCalled  bool
}

func (r *avatarCommunityRepo) GetCommunityByID(context.Context, int64) (*model.Community, error) {
	return r.community, nil
}

func (r *avatarCommunityRepo) SaveCommunityWithRoom(_ context.Context, name, description string, avatar *repository.UpdateCommunityAvatarParam, _ int64) (*model.Community, error) {
	r.savedAvatar = avatar
	return &model.Community{Name: name, Description: description}, nil
}

func (r *avatarCommunityRepo) UpdateCommunity(_ context.Context, _ *model.Community, avatar *repository.UpdateCommunityAvatarParam) error {
	r.updateCalled = true
	r.updatedAvatar = avatar
	return nil
}

func avatarContext(userID int64) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{ID: userID, Role: "user"})
}

type avatarRoomUserRepo struct{ repository.RoomUserRepository }

func (avatarRoomUserRepo) GetRoomUserRole(context.Context, int64, int64) (string, error) {
	return model.RoomUserRoleOwner, nil
}

type nonOwnerRoomUserRepo struct{ repository.RoomUserRepository }

func (nonOwnerRoomUserRepo) GetRoomUserRole(context.Context, int64, int64) (string, error) {
	return model.RoomUserRoleMember, nil
}

func TestCreateCommunityRejectsAnotherUsersAvatarKey(t *testing.T) {
	repo := &avatarCommunityRepo{}
	uc := NewCreateCommunityUseCase(repo)
	key := "community-icons/99/avatar.webp"
	if _, err := uc.Execute(avatarContext(42), "name", "description", &key); err == nil {
		t.Fatal("another user's avatar key must be rejected")
	}
	if repo.savedAvatar != nil {
		t.Fatal("repository must not be called with an invalid avatar key")
	}
}

func TestCreateCommunityPassesOwnedAvatarToAtomicSave(t *testing.T) {
	repo := &avatarCommunityRepo{}
	uc := NewCreateCommunityUseCase(repo)
	key := "community-icons/42/avatar.webp"
	if _, err := uc.Execute(avatarContext(42), "name", "description", &key); err != nil {
		t.Fatal(err)
	}
	if repo.savedAvatar == nil || repo.savedAvatar.StorageKey != key || repo.savedAvatar.UploaderUserID != 42 {
		t.Fatalf("unexpected avatar param: %#v", repo.savedAvatar)
	}
}

func TestUpdateCommunityNoneClearsAvatar(t *testing.T) {
	c := &model.Community{ID: 5, RoomID: 8, AvatarMedia: &model.Media{ID: 1}}
	repo := &avatarCommunityRepo{community: c}
	uc := NewUpdateCommunityUseCase(repo, avatarRoomUserRepo{})
	key := "none"
	if _, err := uc.Execute(avatarContext(42), c.ID, UpdateCommunityParam{AvatarKey: &key}); err != nil {
		t.Fatal(err)
	}
	if !repo.updateCalled || repo.updatedAvatar != nil || c.AvatarMedia != nil {
		t.Fatalf("avatar was not cleared: community=%#v param=%#v", c.AvatarMedia, repo.updatedAvatar)
	}
}

func TestUpdateCommunityRejectsNonOwnerInsideUseCase(t *testing.T) {
	c := &model.Community{ID: 5, RoomID: 8, Name: "before"}
	repo := &avatarCommunityRepo{community: c}
	uc := NewUpdateCommunityUseCase(repo, nonOwnerRoomUserRepo{})
	name := "after"

	if _, err := uc.Execute(avatarContext(42), c.ID, UpdateCommunityParam{Name: &name}); err == nil {
		t.Fatal("non-owner must be rejected")
	}
	if repo.updateCalled || c.Name != "before" {
		t.Fatal("authorization must happen before mutating or saving the community")
	}
}
