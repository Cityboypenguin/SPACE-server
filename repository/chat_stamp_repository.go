package repository

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/model"
)

type ChatStampRepository interface {
	List(context.Context) ([]*model.ChatStamp, error)
	Get(context.Context, string) (*model.ChatStamp, error)
	Create(context.Context, *model.ChatStamp) error
	// Update compares Version and increments it; false means a concurrent change.
	Update(context.Context, *model.ChatStamp) (bool, error)
}
