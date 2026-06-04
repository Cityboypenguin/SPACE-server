package repository

import "context"

type MaintenanceRepository interface {
	SetMaintenance(ctx context.Context, enabled bool) error
	IsMaintenance(ctx context.Context) (bool, error)
}
