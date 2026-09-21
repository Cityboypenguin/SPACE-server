package terms

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	uploadusecase "github.com/Cityboypenguin/SPACE-server/usecase/upload"
)

type CreateTermsInput struct {
	Version       string
	ObjectKey     string
	EffectiveDate time.Time
}

type CreateTermsUseCase struct {
	uploads   uploadusecase.Acceptor
	termsRepo repository.TermsRepository
}

func NewCreateTermsUseCase(uploads uploadusecase.Acceptor, termsRepo repository.TermsRepository) *CreateTermsUseCase {
	return &CreateTermsUseCase{uploads: uploads, termsRepo: termsRepo}
}

func (u *CreateTermsUseCase) Execute(ctx context.Context, input CreateTermsInput) (_ *model.TermsOfService, err error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	version := strings.TrimSpace(input.Version)
	// 受け入れはここで通す（リゾルバの手順にしない理由は usecase/upload 参照）。
	// 保存が成立しなければ、公開した実体は取り消す。
	uploads := uploadusecase.Begin(u.uploads)
	defer uploads.DiscardOnError(ctx, &err)

	objectKey, err := uploads.Accept(ctx, uploadusecase.TermsDocument, strings.TrimSpace(input.ObjectKey))
	if err != nil {
		return nil, err
	}

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
