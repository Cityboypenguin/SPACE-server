package community

import (
	"context"
	"testing"

	"github.com/Cityboypenguin/SPACE-server/internal/auth"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
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

// アイコンキーを「コミュニティアイコンとして」受け入れに通していること。
//
// 持ち主と種別の判定そのものは受け入れ側（internal/upload.CheckKey）が持つ。
// ここで見たいのは、種別を取り違えずに渡しているか。取り違えると、例えば
// 自分のアバターのキーをコミュニティアイコンとして保存できてしまう。
func TestCreateCommunityAcceptsTheKeyAsACommunityIcon(t *testing.T) {
	repo := &avatarCommunityRepo{}
	uploads := &passThroughUploads{}
	uc := NewCreateCommunityUseCase(uploads, repo)
	key := "community-icons/42/avatar.webp"
	if _, err := uc.Execute(avatarContext(42), "name", "description", &key); err != nil {
		t.Fatal(err)
	}
	if len(uploads.kinds) != 1 || uploads.kinds[0] != uploadusecase.CommunityIcon {
		t.Fatalf("受け入れた種別 = %v, want [%q]", uploads.kinds, uploadusecase.CommunityIcon)
	}
}

func TestCreateCommunityPassesOwnedAvatarToAtomicSave(t *testing.T) {
	repo := &avatarCommunityRepo{}
	uc := NewCreateCommunityUseCase(&passThroughUploads{}, repo)
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
	uc := NewUpdateCommunityUseCase(&passThroughUploads{}, repo, avatarRoomUserRepo{})
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
	uc := NewUpdateCommunityUseCase(&passThroughUploads{}, repo, nonOwnerRoomUserRepo{})
	name := "after"

	if _, err := uc.Execute(avatarContext(42), c.ID, UpdateCommunityParam{Name: &name}); err == nil {
		t.Fatal("non-owner must be rejected")
	}
	if repo.updateCalled || c.Name != "before" {
		t.Fatal("authorization must happen before mutating or saving the community")
	}
}

// passThroughUploads は受け入れを素通しする Acceptor。
// 種別・持ち主・上限の判定は受け入れ側（internal/upload）の担当なので、ここでは
// 「どの種別として受け入れを通したか」だけを見る。
type passThroughUploads struct{ kinds []uploadusecase.Kind }

func (p *passThroughUploads) Accept(_ context.Context, kind uploadusecase.Kind, objectKey string) (string, error) {
	p.kinds = append(p.kinds, kind)
	return objectKey, nil
}

func (*passThroughUploads) Discard(context.Context, string) {}

// recordingUploads は受け入れを呼ばれたキーを覚える Acceptor。
type recordingUploads struct{ accepted []string }

func (r *recordingUploads) Accept(_ context.Context, _ uploadusecase.Kind, objectKey string) (string, error) {
	r.accepted = append(r.accepted, objectKey)
	return objectKey, nil
}

func (*recordingUploads) Discard(context.Context, string) {}

// TestUpdateCommunity_RemoveSignalSkipsUploadAcceptance は、アイコンを外す操作が
// アップロードの受け入れに巻き込まれないことを確かめる。
//
// 「外す」はキーの代わりに合図（AvatarKeyRemove）を送る作り。合図は
// アップロードされたオブジェクトではないので、受け入れへ渡すと「そんなキーは無い」
// で必ず弾かれ、アイコンを外す操作ができなくなる。実際そうなっていた。
func TestUpdateCommunity_RemoveSignalSkipsUploadAcceptance(t *testing.T) {
	c := &model.Community{ID: 5, RoomID: 8, AvatarMedia: &model.Media{ID: 1}}
	repo := &avatarCommunityRepo{community: c}
	uploads := &recordingUploads{}
	uc := NewUpdateCommunityUseCase(uploads, repo, avatarRoomUserRepo{})

	remove := AvatarKeyRemove
	if _, err := uc.Execute(avatarContext(42), c.ID, UpdateCommunityParam{AvatarKey: &remove}); err != nil {
		t.Fatalf("アイコンを外す操作が失敗した: %v", err)
	}
	if len(uploads.accepted) != 0 {
		t.Fatalf("受け入れに渡された = %v, want 合図は渡さないこと", uploads.accepted)
	}
}
