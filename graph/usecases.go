package graph

import (
	"github.com/Cityboypenguin/SPACE-server/repository"
	communityusecase "github.com/Cityboypenguin/SPACE-server/usecase/community"
)

// NewCommunityUseCases wires every community use case from its repositories and
// returns the grouped struct. Centralizing the wiring here keeps main.go free of
// the per-use-case construction boilerplate and means adding a community use case
// touches one place instead of three.
func NewCommunityUseCases(
	communityRepo repository.CommunityRepository,
	mediaRepo repository.MediaRepository,
	roomUserRepo repository.RoomUserRepository,
	txManager repository.TxManager,
) CommunityUseCases {
	return CommunityUseCases{
		CreateCommunityUseCase:             communityusecase.NewCreateCommunityUseCase(communityRepo, mediaRepo),
		GetCommunityUseCase:                communityusecase.NewGetCommunityUseCase(communityRepo),
		UpdateCommunityUseCase:             communityusecase.NewUpdateCommunityUseCase(communityRepo),
		UpdateCommunityMembersUseCase:      communityusecase.NewUpdateCommunityMembersUseCase(communityRepo, roomUserRepo, txManager),
		SearchCommunityUseCase:             communityusecase.NewSearchCommunityUseCase(communityRepo),
		ListMyCommunitiesUseCase:           communityusecase.NewListMyCommunitiesUseCase(communityRepo),
		ListAllCommunitiesUseCase:          communityusecase.NewListAllCommunitiesUseCase(communityRepo),
		PromoteToCommunityOwnerUseCase:     communityusecase.NewPromoteToCommunityOwnerUseCase(communityRepo, roomUserRepo),
		DemoteFromCommunityOwnerUseCase:    communityusecase.NewDemoteFromCommunityOwnerUseCase(communityRepo, roomUserRepo),
		IsSoleOwnerWithOtherMembersUseCase: communityusecase.NewIsSoleOwnerWithOtherMembersUseCase(communityRepo),
		GetRandomCommunitiesUseCase:        *communityusecase.NewGetRandomCommunitiesUseCase(communityRepo),
	}
}
