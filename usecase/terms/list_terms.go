package terms

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListTermsUseCase struct {
	TermsRepository repository.TermsRepository
}

func NewListTermsUseCase(r repository.TermsRepository) *ListTermsUseCase {
	return &ListTermsUseCase{TermsRepository: r}
}

func (uc *ListTermsUseCase) Execute(ctx context.Context, q repository.PageQuery) ([]*model.TermsOfService, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return uc.TermsRepository.FindAll(ctx, q)
}
