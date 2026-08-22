// Package anon implements the per-room fixed anonymous identity used to display
// student authors in course chats (F-05) without revealing their real account.
package anon

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type GetOrCreateAnonymousIdentityUseCase interface {
	Execute(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error)
}

var _ GetOrCreateAnonymousIdentityUseCase = &GetOrCreateAnonymousIdentityInteractor{}

type GetOrCreateAnonymousIdentityInteractor struct {
	identityRepo repository.RoomAnonymousIdentityRepository
}

func NewGetOrCreateAnonymousIdentityUseCase(identityRepo repository.RoomAnonymousIdentityRepository) GetOrCreateAnonymousIdentityUseCase {
	return &GetOrCreateAnonymousIdentityInteractor{identityRepo: identityRepo}
}

// Execute is called from the GraphQL layer only after the caller has already been
// authenticated (it is not itself exposed as an API field), so it does not repeat
// an authz.RequireAuth check.
func (uc *GetOrCreateAnonymousIdentityInteractor) Execute(ctx context.Context, roomID, userID int64) (*model.RoomAnonymousIdentity, error) {
	return uc.identityRepo.GetOrCreate(ctx, roomID, userID)
}
