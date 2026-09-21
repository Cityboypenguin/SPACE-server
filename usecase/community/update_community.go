package community

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

type UpdateCommunityParam struct {
	Name        *string
	Description *string
	AvatarKey   *string
}

type UpdateCommunityUseCase interface {
	Execute(ctx context.Context, communityID int64, param UpdateCommunityParam) (*model.Community, error)
}

var _ UpdateCommunityUseCase = &UpdateCommunityInteractor{}

type UpdateCommunityInteractor struct {
	uploads       uploadusecase.Acceptor
	communityRepo repository.CommunityRepository
	roomUserRepo  repository.RoomRoleRepository
}

func NewUpdateCommunityUseCase(uploads uploadusecase.Acceptor, communityRepo repository.CommunityRepository, roomUserRepo repository.RoomRoleRepository) UpdateCommunityUseCase {
	return &UpdateCommunityInteractor{uploads: uploads,
		communityRepo: communityRepo,
		roomUserRepo:  roomUserRepo,
	}
}

func (uc *UpdateCommunityInteractor) Execute(ctx context.Context, communityID int64, param UpdateCommunityParam) (_ *model.Community, err error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	c, err := uc.communityRepo.GetCommunityByID(ctx, communityID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("community not found")
	}
	if !authz.IsAdminRole(claims.Role) {
		role, err := uc.roomUserRepo.GetRoomUserRole(ctx, c.RoomID, claims.ID)
		if err != nil {
			return nil, err
		}
		if role != model.RoomUserRoleOwner {
			return nil, errors.New("forbidden: only community owners or administrators can update the community")
		}
	}
	// 保存が成立しなければ、受け入れで公開した実体は取り消す。
	uploads := uploadusecase.Begin(uc.uploads)
	defer uploads.DiscardOnError(ctx, &err)

	c.UpdateCommunity(model.UpdateCommunityParam{Name: param.Name, Description: param.Description})

	var avatar *repository.UpdateCommunityAvatarParam
	if param.AvatarKey != nil && *param.AvatarKey == AvatarKeyRemove {
		c.AvatarMedia = nil
	} else if param.AvatarKey != nil && *param.AvatarKey != "" {
		// 受け入れはここで通す（リゾルバの手順にしない理由は usecase/upload 参照）。
		// 種別と所有者はその中で、ストレージに触る前に確かめられる。
		accepted, err := uploads.Accept(ctx, uploadusecase.CommunityIcon, *param.AvatarKey)
		if err != nil {
			return nil, err
		}
		param.AvatarKey = &accepted
		avatar = &repository.UpdateCommunityAvatarParam{
			UploaderUserID: claims.ID,
			StorageKey:     *param.AvatarKey,
			ContentType:    contentTypeFromKey(*param.AvatarKey),
		}
	}

	if err := uc.communityRepo.UpdateCommunity(ctx, c, avatar); err != nil {
		return nil, err
	}
	return c, nil
}
