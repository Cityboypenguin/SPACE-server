package profile

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

type SetAvatarUseCase interface {
	Execute(ctx context.Context, objectKey string) (*model.Profile, error)
}

type SetAvatarInteractor struct {
	uploads     uploadusecase.Acceptor
	profileRepo repository.ProfileRepository
	mediaRepo   repository.MediaWriter
	txManager   repository.TxManager
}

func NewSetAvatarUseCase(uploads uploadusecase.Acceptor, profileRepo repository.ProfileRepository, mediaRepo repository.MediaWriter, txManager repository.TxManager) SetAvatarUseCase {
	return &SetAvatarInteractor{uploads: uploads, profileRepo: profileRepo, mediaRepo: mediaRepo, txManager: txManager}
}

func (uc *SetAvatarInteractor) Execute(ctx context.Context, objectKey string) (_ *model.Profile, err error) {
	userID, err := authz.CallerID(ctx)
	if err != nil {
		return nil, err
	}
	// 受け入れはここで通す。リゾルバの手順にしておくと、このユースケースを
	// 直接呼ぶ経路が増えたときに検査を飛ばせてしまう。
	//
	// 種別と所有者の確認は受け入れの中で、ストレージに触る前に済む
	// （internal/upload.CheckKey）。保存が成立しなければ、公開した実体は取り消す。
	uploads := uploadusecase.Begin(uc.uploads)
	defer uploads.DiscardOnError(ctx, &err)

	objectKey, err = uploads.Accept(ctx, uploadusecase.Avatar, objectKey)
	if err != nil {
		return nil, err
	}

	media := &model.Media{
		UploaderUserID: userID,
		StorageKey:     objectKey,
		ContentType:    contentTypeFromKey(objectKey),
		CreatedAt:      time.Now(),
	}
	var p *model.Profile
	err = uc.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if err := uc.mediaRepo.CreateMedia(txCtx, media); err != nil {
			return err
		}

		if err := uc.profileRepo.SetAvatarMedia(txCtx, userID, media.ID); err != nil {
			return err
		}

		var err error
		p, err = uc.profileRepo.GetProfileByUserID(txCtx, userID)
		return err
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

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
