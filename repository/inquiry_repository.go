package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type InquiryRepository interface {
	Save(ctx context.Context, inquiry *model.Inquiry) error
	FindAll(ctx context.Context, status *model.InquiryStatus) ([]*model.Inquiry, error)
	FindByID(ctx context.Context, id string) (*model.Inquiry, error)
	UpdateStatus(ctx context.Context, id string, status model.InquiryStatus) (*model.Inquiry, error)
}
