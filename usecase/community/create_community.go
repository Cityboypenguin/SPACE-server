package community

import (
	"context"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

type CreateCommunityUseCase interface {
	Execute(ctx context.Context, name, description string, avatarKey *string) (*model.Community, error)
}

var _ CreateCommunityUseCase = &CreateCommunityInteractor{}

type CreateCommunityInteractor struct {
	uploads       uploadusecase.Acceptor
	communityRepo repository.CommunityRepository
}

func NewCreateCommunityUseCase(uploads uploadusecase.Acceptor, communityRepo repository.CommunityRepository) CreateCommunityUseCase {
	return &CreateCommunityInteractor{uploads: uploads, communityRepo: communityRepo}
}

func (uc *CreateCommunityInteractor) Execute(ctx context.Context, name, description string, avatarKey *string) (_ *model.Community, err error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// 保存が成立しなければ、受け入れで公開した実体は取り消す。
	uploads := uploadusecase.Begin(uc.uploads)
	defer uploads.DiscardOnError(ctx, &err)

	var avatar *repository.UpdateCommunityAvatarParam
	if avatarKey != nil && *avatarKey != "" {
		// 受け入れはここで通す（リゾルバの手順にしない理由は usecase/upload 参照）。
		// 種別と所有者はその中で、ストレージに触る前に確かめられる。
		accepted, err := uploads.Accept(ctx, uploadusecase.CommunityIcon, *avatarKey)
		if err != nil {
			return nil, err
		}
		avatarKey = &accepted
		avatar = &repository.UpdateCommunityAvatarParam{
			UploaderUserID: claims.ID,
			StorageKey:     *avatarKey,
			ContentType:    contentTypeFromKey(*avatarKey),
		}
	}

	c, err := uc.communityRepo.SaveCommunityWithRoom(ctx, name, description, avatar, claims.ID)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// AvatarKeyRemove は avatarKey の代わりに送る「アイコンを外す」の合図。
//
// アップロードされたオブジェクトのキーではないので、受け入れ（upload.Accept）を
// 通してはいけない。通すと「そんなキーは無い」として弾かれ、アイコンを外す操作が
// できなくなる。公開しているのは、合図かどうかを判断する側（graph）と
// 解釈する側（ここ）で同じ文字列を使うため。
const AvatarKeyRemove = "none"

func contentTypeFromKey(key string) string {
	switch {
	case strings.HasSuffix(key, ".jpg"):
		return "image/jpeg"
	case strings.HasSuffix(key, ".png"):
		return "image/png"
	case strings.HasSuffix(key, ".webp"):
		return "image/webp"
	case strings.HasSuffix(key, ".gif"):
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}
