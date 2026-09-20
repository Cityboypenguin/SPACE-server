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
	// Execute は userID の時間割と、それが呼び出し元に見えるかどうかを返す。
	//
	// 閲覧可否（公開設定 setTimetableProfileVisibility とブロック関係）の判定は
	// ここに集約してある。呼び出し側で同じ判定をやり直してはいけない（以前は
	// リゾルバが IsProfileVisible を呼んだうえで Execute が内部で同じ判定を
	// もう一度していた）。visible が必要なのは「非公開です」と表示を出し分ける
	// GraphQL フィールドがあるためで、判定そのものはユースケースの責務。
	//
	// visible が false のとき entries は必ず空。
	Execute(ctx context.Context, userID int64, year *int, semesterName *string) (entries []*repository.TimetableEntryWithCourse, visible bool, err error)
}

var _ GetUserTimetableUseCase = &GetUserTimetableInteractor{}

type GetUserTimetableInteractor struct {
	timetableRepo   repository.TimetableRepository
	settingRepo     repository.SystemSettingRepository
	userSettingRepo repository.UserSettingRepository
	blockRepo       repository.BlockerRepository
}

func NewGetUserTimetableUseCase(timetableRepo repository.TimetableRepository, settingRepo repository.SystemSettingRepository, userSettingRepo repository.UserSettingRepository, blockRepo repository.BlockerRepository) GetUserTimetableUseCase {
	return &GetUserTimetableInteractor{timetableRepo: timetableRepo, settingRepo: settingRepo, userSettingRepo: userSettingRepo, blockRepo: blockRepo}
}

// Execute returns userID's timetable for the given year/semester, defaulting to the
// current semester when both are omitted (same rule as ListTimetableUseCase),
// together with whether the caller is allowed to see it. The owner themselves and
// admins always see it; for anyone else it is hidden when userID turned the profile
// timetable off via setTimetableProfileVisibility, or when either user has blocked
// the other. When it is hidden, no entries are loaded and visible is false.
func (uc *GetUserTimetableInteractor) Execute(ctx context.Context, userID int64, year *int, semesterName *string) ([]*repository.TimetableEntryWithCourse, bool, error) {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return nil, false, err
	}

	if claims.ID != userID && !authz.IsAdminRole(claims.Role) {
		visible, err := uc.isProfileVisible(ctx, claims.ID, userID)
		if err != nil {
			return nil, false, err
		}
		if !visible {
			return nil, false, nil
		}
	}

	y, s := 0, ""
	if year != nil && semesterName != nil {
		y, s = *year, *semesterName
	} else {
		y, s, err = semester.Get(ctx, uc.settingRepo)
		if err != nil {
			return nil, false, err
		}
	}

	entries, err := uc.timetableRepo.ListByUser(ctx, userID, y, s)
	if err != nil {
		return nil, false, err
	}
	return entries, true, nil
}

// isProfileVisible reports whether viewerID may see userID's timetable (visibility
// defaults to true when no preference has been saved). Execute is the only caller:
// 可否判定をユースケースの外に出さないため、非公開メソッドにしてある。
func (uc *GetUserTimetableInteractor) isProfileVisible(ctx context.Context, viewerID, userID int64) (bool, error) {
	blocked, err := uc.blockRepo.ExistsBlockRelation(ctx, viewerID, userID)
	if err != nil {
		return false, err
	}
	if blocked {
		return false, nil
	}

	value, found, err := uc.userSettingRepo.Get(ctx, userID, usersettingsusecase.TimetableProfileVisibilityKey)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	return strconv.ParseBool(value)
}
