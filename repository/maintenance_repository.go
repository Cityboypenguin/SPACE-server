package repository

import "context"

type MaintenanceRepository interface {
	IsMaintenanceModeEnabled(ctx context.Context) (bool, error)
	SetMaintenanceMode(ctx context.Context, enabled bool) error
}
