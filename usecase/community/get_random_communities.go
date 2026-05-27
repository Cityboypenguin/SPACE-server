package community

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetRandomCommunitiesUseCase struct {
    CommunityRepo repository.CommunityRepository
}

func NewGetRandomCommunitiesUseCase(repo repository.CommunityRepository) *GetRandomCommunitiesUseCase {
    return &GetRandomCommunitiesUseCase{
        CommunityRepo: repo,
    }
}

func (u *GetRandomCommunitiesUseCase) Execute(ctx context.Context, limit int) ([]*model.Community, error) {
    return u.CommunityRepo.FindRandom(ctx, limit)
}