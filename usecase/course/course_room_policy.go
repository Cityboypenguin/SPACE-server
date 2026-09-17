package course

import (
	"context"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/semester"
)

// courseRoomWritePolicy は「その授業ルームに今書き込んでよいか」(F-06) の判定本体。
//
// この判定を使うのは CheckRoomWritableUseCase（メッセージ用。授業ルームでなければ
// 素通し）と RequireWritableCourseRoomUseCase（質問・回答・投票用。授業ルームで
// なければ拒否）の2つで、違うのは「授業ルームでなかったときにどう振る舞うか」だけ。
// 学期判定と履修判定を両方に書き写すと片方だけ直す事故が起きるので、判定は
// ここ1箇所に置き、上の2つはその薄いラッパにしている。
type courseRoomWritePolicy struct {
	courseRepo    repository.CourseRepository
	settingRepo   repository.SystemSettingRepository
	timetableRepo repository.TimetableRepository
}

// ensureWritable は roomID が授業ルームならその Course を返す。書き込めない授業
// ルームならエラーを返し、そもそも授業ルームでなければ (nil, nil) を返して
// 判断を呼び出し側に委ねる。
func (p courseRoomWritePolicy) ensureWritable(ctx context.Context, roomID int64) (*model.Course, error) {
	c, err := p.courseRepo.GetCourseByRoomID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}

	if err := p.ensureCurrentSemester(ctx, c); err != nil {
		return nil, err
	}
	if err := p.ensureRegistered(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ensureCurrentSemester は学期によるアーカイブ判定。
//
// 通年 (full-year) courses span both semesters of their year, so they're never
// archived by a semester switch within the same year - only a year change ends them.
func (p courseRoomWritePolicy) ensureCurrentSemester(ctx context.Context, c *model.Course) error {
	year, semesterName, err := semester.Get(ctx, p.settingRepo)
	if err != nil {
		return err
	}
	if c.Year == year && (c.Semester == semesterName || c.Semester == model.SemesterFull) {
		return nil
	}
	return apperr.Forbidden("この授業は現在の学期の対象外のため、閲覧のみ可能です")
}

// ensureRegistered は履修（時間割登録）判定。書き込めるのは登録済みの学生だけで、
// 閲覧は誰でもできる（F-04）。管理者は登録なしでも書き込める。
func (p courseRoomWritePolicy) ensureRegistered(ctx context.Context, c *model.Course) error {
	claims, err := authz.RequireAuth(ctx)
	if err != nil {
		return err
	}
	if authz.IsAdminRole(claims.Role) {
		return nil
	}

	registered, err := p.timetableRepo.IsRegistered(ctx, claims.ID, c.ID)
	if err != nil {
		return err
	}
	if !registered {
		return apperr.Forbidden("この授業を時間割に登録していないため、書き込みできません。")
	}
	return nil
}
