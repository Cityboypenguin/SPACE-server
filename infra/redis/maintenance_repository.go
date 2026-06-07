package redis

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/redis/go-redis/v9"
)

const maintenanceModeKey = "maintenance:enabled"

type RedisMaintenanceRepository struct {
	client *redis.Client
}

func NewRedisMaintenanceRepository(client *redis.Client) repository.MaintenanceRepository {
	return &RedisMaintenanceRepository{client: client}
}

func (r *RedisMaintenanceRepository) IsMaintenanceModeEnabled(ctx context.Context) (bool, error) {
	val, err := r.client.Get(ctx, maintenanceModeKey).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == "true", nil
}

func (r *RedisMaintenanceRepository) SetMaintenanceMode(ctx context.Context, enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return r.client.Set(ctx, maintenanceModeKey, val, 0).Err()
}
