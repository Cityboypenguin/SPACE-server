package inquiry

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/google/uuid"
)

var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

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
	name := strings.TrimSpace(input.Name)
	email := strings.TrimSpace(input.Email)
	subject := strings.TrimSpace(input.Subject)
	content := strings.TrimSpace(input.Content)

	if name == "" || email == "" || subject == "" || content == "" {
		return nil, errors.New("all fields are required")
	}
	if !emailRegex.MatchString(email) {
		return nil, errors.New("invalid email format")
	}
	if len(name) > 255 {
		return nil, errors.New("name must be 255 characters or less")
	}
	if len(email) > 255 {
		return nil, errors.New("email must be 255 characters or less")
	}
	if len(subject) > 255 {
		return nil, errors.New("subject must be 255 characters or less")
	}
	if len(content) > 10000 {
		return nil, errors.New("content must be 10000 characters or less")
	}

	inquiry := &model.Inquiry{
		ID:        uuid.New().String(),
		Name:      name,
		Email:     email,
		Subject:   subject,
		Content:   content,
		CreatedAt: time.Now(),
	}

	if err := u.inquiryRepo.Save(ctx, inquiry); err != nil {
		return nil, err
	}

	return inquiry, nil
}
