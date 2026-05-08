package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

type IsSoleOwnerWithOtherMembersUseCase interface {
	Execute(ctx context.Context, userID int64) (bool, error)
}

var _ IsSoleOwnerWithOtherMembersUseCase = &IsSoleOwnerWithOtherMembersInteractor{}

type IsSoleOwnerWithOtherMembersInteractor struct {
	communityRepo repository.CommunityRepository
}

func NewIsSoleOwnerWithOtherMembersUseCase(communityRepo repository.CommunityRepository) IsSoleOwnerWithOtherMembersUseCase {
	return &IsSoleOwnerWithOtherMembersInteractor{communityRepo: communityRepo}
}

func (uc *IsSoleOwnerWithOtherMembersInteractor) Execute(ctx context.Context, userID int64) (bool, error) {
	return uc.communityRepo.IsSoleOwnerWithOtherMembers(ctx, userID)
}
