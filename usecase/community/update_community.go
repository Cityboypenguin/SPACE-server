package community

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
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
	communityRepo repository.CommunityRepository
	roomUserRepo  repository.RoomUserRepository
}

func NewUpdateCommunityUseCase(communityRepo repository.CommunityRepository, roomUserRepo repository.RoomUserRepository) UpdateCommunityUseCase {
	return &UpdateCommunityInteractor{
		communityRepo: communityRepo,
		roomUserRepo:  roomUserRepo,
	}
}

func (uc *UpdateCommunityInteractor) Execute(ctx context.Context, communityID int64, param UpdateCommunityParam) (*model.Community, error) {
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
	c.UpdateCommunity(model.UpdateCommunityParam{Name: param.Name, Description: param.Description})

	var avatar *repository.UpdateCommunityAvatarParam
	if param.AvatarKey != nil && *param.AvatarKey == "none" {
		c.AvatarMedia = nil
	} else if param.AvatarKey != nil && *param.AvatarKey != "" {
		if err := validateCommunityAvatarKey(claims.ID, *param.AvatarKey); err != nil {
			return nil, err
		}
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
