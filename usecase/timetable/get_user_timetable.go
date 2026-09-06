package timetable

import (
	"context"
	"strconv"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
	usersettingsusecase "github.com/Cityboypenguin/SPACE-server/usecase/user_settings"
)

type GetUserTimetableUseCase interface {
	Execute(ctx context.Context, userID int64, year *int, semesterName *string) ([]*repository.TimetableEntryWithCourse, error)
	// IsProfileVisible reports whether userID has made their timetable visible to
	// other users on their profile (true when the caller is userID themselves,
	// since this is only meant to gate visibility to other users).
	IsProfileVisible(ctx context.Context, userID int64) (bool, error)
}

var _ GetUserTimetableUseCase = &GetUserTimetableInteractor{}

type GetUserTimetableInteractor struct {
	timetableRepo   repository.TimetableRepository
	settingRepo     repository.SystemSettingRepository
	userSettingRepo repository.UserSettingRepository
}

func NewGetUserTimetableUseCase(timetableRepo repository.TimetableRepository, settingRepo repository.SystemSettingRepository, userSettingRepo repository.UserSettingRepository) GetUserTimetableUseCase {
	return &GetUserTimetableInteractor{timetableRepo: timetableRepo, settingRepo: settingRepo, userSettingRepo: userSettingRepo}
}

// Execute returns userID's timetable for the given year/semester, defaulting to the
// current semester when both are omitted (same rule as ListTimetableUseCase). When
// the caller is neither userID nor an admin, an empty slice is returned instead of
// the real entries if userID has hidden their timetable from their profile via
// setTimetableProfileVisibility.
func (uc *GetUserTimetableInteractor) Execute(ctx context.Context, userID int64, year *int, semesterName *string) ([]*repository.TimetableEntryWithCourse, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if claims.ID != userID && !authz.IsAdminRole(claims.Role) {
		visible, err := uc.isProfileVisible(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, nil
		}
	}

	y, s := 0, ""
	if year != nil && semesterName != nil {
		y, s = *year, *semesterName
	} else {
		y, s, err = semester.Get(ctx, uc.settingRepo)
		if err != nil {
			return nil, err
		}
	}

	return uc.timetableRepo.ListByUser(ctx, userID, y, s)
}

// IsProfileVisible reports whether userID's timetable is visible to other users
// on their profile (defaults to true when no preference has been saved).
func (uc *GetUserTimetableInteractor) IsProfileVisible(ctx context.Context, userID int64) (bool, error) {
	return uc.isProfileVisible(ctx, userID)
}

func (uc *GetUserTimetableInteractor) isProfileVisible(ctx context.Context, userID int64) (bool, error) {
	value, found, err := uc.userSettingRepo.Get(ctx, userID, usersettingsusecase.TimetableProfileVisibilityKey)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	return strconv.ParseBool(value)
}
