package redis

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

const maintenanceKey = "system:maintenance"

var _ repository.MaintenanceRepository = &RedisMaintenanceRepository{}

type RedisMaintenanceRepository struct {
	client *redis.Client
}

func NewRedisMaintenanceRepository(client *redis.Client) *RedisMaintenanceRepository {
	return &RedisMaintenanceRepository{client: client}
}

func (r *RedisMaintenanceRepository) SetMaintenance(ctx context.Context, enabled bool) error {
	if enabled {
		return r.client.Set(ctx, maintenanceKey, "1", 0).Err()
	}
	return r.client.Del(ctx, maintenanceKey).Err()
}

func (r *RedisMaintenanceRepository) IsMaintenance(ctx context.Context) (bool, error) {
	result, err := r.client.Exists(ctx, maintenanceKey).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}
