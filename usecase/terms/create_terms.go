package terms

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type CreateTermsInput struct {
	Version       string
	ObjectKey     string
	EffectiveDate time.Time
}

type CreateTermsUseCase struct {
	termsRepo repository.TermsRepository
}

func NewCreateTermsUseCase(termsRepo repository.TermsRepository) *CreateTermsUseCase {
	return &CreateTermsUseCase{termsRepo: termsRepo}
}

func (u *CreateTermsUseCase) Execute(ctx context.Context, input CreateTermsInput) (*model.TermsOfService, error) {
	version := strings.TrimSpace(input.Version)
	objectKey := strings.TrimSpace(input.ObjectKey)

	if version == "" || objectKey == "" {
		return nil, errors.New("version and objectKey are required")
	}
	if input.EffectiveDate.IsZero() {
		return nil, errors.New("effectiveDate is required")
	}

	t := &model.TermsOfService{
		Version:       version,
		ObjectKey:     objectKey,
		EffectiveDate: input.EffectiveDate,
		CreatedAt:     time.Now(),
	}

	if err := u.termsRepo.Save(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}
