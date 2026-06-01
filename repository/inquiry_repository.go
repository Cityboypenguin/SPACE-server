package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type InquiryRepository interface {
	Save(ctx context.Context, inquiry *model.Inquiry) error
}
