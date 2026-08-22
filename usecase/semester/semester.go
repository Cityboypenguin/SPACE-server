// Package semester manages the "current semester" (year + 前期/後期) that
// timetable defaults and course-chat archive checks are based on. Instead of a
// dedicated table, the value is stored as a single system_settings row
// (see repository.SystemSettingRepository), keyed by SettingKey.
package semester

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/repository"
)

const SettingKey = "current_semester"

// Get returns the current year and semester ("前期"/"後期") stored in system_settings.
func Get(ctx context.Context, settingRepo repository.SystemSettingRepository) (year int, semesterName string, err error) {
	value, err := settingRepo.Get(ctx, SettingKey)
	if err != nil {
		return 0, "", err
	}

	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid %s value: %q", SettingKey, value)
	}

	y, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid %s year: %w", SettingKey, err)
	}

	return y, parts[1], nil
}

// Set stores the current year and semester in system_settings.
func Set(ctx context.Context, settingRepo repository.SystemSettingRepository, year int, semesterName string, updatedAt int64) error {
	return settingRepo.Update(ctx, SettingKey, fmt.Sprintf("%d:%s", year, semesterName), updatedAt)
}
