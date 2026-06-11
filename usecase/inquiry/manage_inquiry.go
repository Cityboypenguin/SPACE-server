package inquiry

import (
	"context"
	"errors"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
)

type ManageInquiryUsecase struct {
	inquiryRepo repository.InquiryRepository
}

func NewManageInquiryUsecase(inquiryRepo repository.InquiryRepository) *ManageInquiryUsecase {
	return &ManageInquiryUsecase{inquiryRepo: inquiryRepo}
}

func (u *ManageInquiryUsecase) Search(ctx context.Context, status *model.InquiryStatus, limit, offset int) ([]*model.Inquiry, int, error) {
	return u.inquiryRepo.FindAll(ctx, status, limit, offset)
}

func (u *ManageInquiryUsecase) GetByID(ctx context.Context, id string) (*model.Inquiry, error) {
	if id == "" {
		return nil, errors.New("inquiry ID is required")
	}
	return u.inquiryRepo.FindByID(ctx, id)
}

func (u *ManageInquiryUsecase) UpdateStatus(ctx context.Context, id string, status model.InquiryStatus) (*model.Inquiry, error) {
	if id == "" {
		return nil, errors.New("inquiry ID is required")
	}
	return u.inquiryRepo.UpdateStatus(ctx, id, status)
}
