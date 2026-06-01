package inquiry

import (
	"context"
	"errors"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/google/uuid"
)

type CreateInquiryUsecase struct {
	inquiryRepo repository.InquiryRepository
}

func NewCreateInquiryUsecase(inquiryRepo repository.InquiryRepository) *CreateInquiryUsecase {
	return &CreateInquiryUsecase{inquiryRepo: inquiryRepo}
}

type CreateInquiryInput struct {
	Name    string
	Email   string
	Subject string
	Content string
}

func (u *CreateInquiryUsecase) Execute(ctx context.Context, input CreateInquiryInput) (*model.Inquiry, error) {
	if input.Name == "" || input.Email == "" || input.Subject == "" || input.Content == "" {
		return nil, errors.New("all fields are required")
	}

	inquiry := &model.Inquiry{
		ID:        uuid.New().String(),
		Name:      input.Name,
		Email:     input.Email,
		Subject:   input.Subject,
		Content:   input.Content,
		CreatedAt: time.Now(),
	}

	if err := u.inquiryRepo.Save(ctx, inquiry); err != nil {
		return nil, err
	}

	return inquiry, nil
}
