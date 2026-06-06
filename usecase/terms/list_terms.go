package terms

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ListTermsUseCase struct {
	TermsRepository repository.TermsRepository
}

func NewListTermsUseCase(r repository.TermsRepository) *ListTermsUseCase {
	return &ListTermsUseCase{TermsRepository: r}
}

func (uc *ListTermsUseCase) Execute(ctx context.Context) ([]*model.TermsOfService, error) {
	return uc.TermsRepository.FindAll(ctx)
}
