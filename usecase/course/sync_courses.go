package course

import (
	"context"
	"fmt"
	"time"

	"github.com/Cityboypenguin/SPACE-server/internal/authz"
	"github.com/Cityboypenguin/SPACE-server/internal/logger"
	"github.com/Cityboypenguin/SPACE-server/model"
	"github.com/Cityboypenguin/SPACE-server/repository"
	"github.com/Cityboypenguin/SPACE-server/usecase/notification"
)

// ScrapedCourseInput is the shape any course data source (scraper, CSV import, ...)
// must produce. Keeping it decoupled from the scraping mechanism means the parser
// can be swapped without touching the sync logic.
type ScrapedCourseInput struct {
	DayOfWeek   string
	Period      int
	TeacherName string
	// CourseName は表示名。同名の授業を見分けるための校舎サフィックスが付くことがある。
	CourseName string
	Year       int
	Semester   string
	DedupKey   string
	// SourceRef は取り込み元での識別子（専修大学なら講義コード）。照合に使う。
	SourceRef string
	// SourceName はサフィックスを付ける前の授業名。名前が変わったかの比較に使う。
	SourceName string
	// Disambiguated はスクレイパーが今回詳細ページまで見て CourseName（サフィックスの
	// 有無）を確かめた行。false の行の CourseName はサフィックスを省いている
	// ことがある（nextCourseName のコメント参照）。
	Disambiguated bool
}

// SyncCoursesInput は同期1回ぶんの入力。
type SyncCoursesInput struct {
	Year         int
	DryRun       bool
	Courses      []ScrapedCourseInput
	Completeness FetchCompleteness
	StartedAt    time.Time
}

type SyncCoursesUseCase interface {
	Execute(ctx context.Context, input SyncCoursesInput) (*model.CourseSyncRun, error)
}

var _ SyncCoursesUseCase = &SyncCoursesInteractor{}

type SyncCoursesInteractor struct {
	courseRepo repository.CourseRepository
	syncRepo   repository.CourseSyncRepository
	txManager  repository.TxManager
	notifier   notification.NotificationPublisher
	now        func() time.Time
}

func NewSyncCoursesUseCase(courseRepo repository.CourseRepository, syncRepo repository.CourseSyncRepository, txManager repository.TxManager, notifier notification.NotificationPublisher) SyncCoursesUseCase {
	return &SyncCoursesInteractor{courseRepo: courseRepo, syncRepo: syncRepo, txManager: txManager, notifier: notifier, now: time.Now}
}

// dryRunSavepoint はドライランで授業への書き込みを巻き戻す地点の名前。
const dryRunSavepoint = "course_sync_dry_run"

// Execute はシラバスの内容（input.Courses）に DB の授業を合わせ、何をしたかを
// course_sync_runs / course_sync_changes に残す。
//
// 書き込みは全部を1つのトランザクションに入れる。途中で失敗したら何も残らない
// （同期は再実行できるので、部分適用を残すより後始末が要らない）。
//
// 時間割は1コマ1授業。授業のコマ（や学期）が変わって、移った先のコマに別の授業を
// 登録している人がいたら、その人の時間割から「移ってきた方」を外す。本人が選んで
// そのコマに入れていた授業を残すため。外した人にはアプリ内通知で知らせる。
//
// DryRun のときも授業への書き込みは本実行と同じ順に行い、記録を残す前に
// セーブポイントまで巻き戻す。こうすると「何人の登録が外れるか」のように書いてみないと
// 分からない数まで、本実行と同じ判定で見積もれる。確認の作成・判断の反映・通知はしない。
//
// 授業の行を DELETE することは無い。廃止は印を付けるだけで、ルーム・メッセージは残る。
// 消えるのは上に書いた、コマの重なりで外す時間割の登録だけ。
func (uc *SyncCoursesInteractor) Execute(ctx context.Context, input SyncCoursesInput) (*model.CourseSyncRun, error) {
	claims, err := authz.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	existing, err := uc.syncRepo.ListSyncableCourses(ctx, input.Year)
	if err != nil {
		return nil, err
	}
	reviews, err := uc.syncRepo.ListReviewsByYear(ctx, input.Year)
	if err != nil {
		return nil, err
	}
	plan := PlanCourseSync(input.Year, existing, input.Courses, reviews, input.Completeness)

	triggeredBy := claims.ID
	run := &model.CourseSyncRun{
		Year:                     input.Year,
		DryRun:                   input.DryRun,
		TriggeredBy:              &triggeredBy,
		SiteTotal:                input.Completeness.SiteTotal,
		ListedRows:               input.Completeness.ListedRows,
		UnidentifiedRows:         input.Completeness.UnidentifiedRows,
		Created:                  len(plan.Creates),
		Discontinued:             len(plan.Discontinues),
		Reviews:                  len(plan.Reviews),
		Unchanged:                plan.Unchanged,
		DiscontinueSkippedReason: plan.DiscontinueSkippedReason,
		StartedAt:                input.StartedAt,
	}
	for _, u := range plan.Updates {
		switch {
		case u.Silent:
		case u.Restore:
			run.Restored++
		default:
			run.Updated++
		}
	}

	var conflicts []repository.SlotConflict
	if err := uc.txManager.RunInTx(ctx, func(ctx context.Context) error {
		now := uc.now()

		// 登録人数は書き換える前に数える（重なりで外す前の人数を出すため）。
		registered, err := uc.syncRepo.CountRegistrations(ctx, affectedCourseIDs(plan))
		if err != nil {
			return err
		}

		if input.DryRun {
			if err := uc.syncRepo.Savepoint(ctx, dryRunSavepoint); err != nil {
				return err
			}
		}
		createdIDs, err := uc.applyCourses(ctx, plan, now)
		if err != nil {
			return err
		}
		conflicts, err = uc.syncRepo.FindSlotConflicts(ctx, movedCourseIDs(plan))
		if err != nil {
			return err
		}
		entryIDs := make([]int64, len(conflicts))
		for i, c := range conflicts {
			entryIDs[i] = c.TimetableID
		}
		if err := uc.syncRepo.DeleteTimetableEntries(ctx, entryIDs); err != nil {
			return err
		}
		if input.DryRun {
			if err := uc.syncRepo.RollbackToSavepoint(ctx, dryRunSavepoint); err != nil {
				return err
			}
			createdIDs = nil // 巻き戻したので、採番した ID はもう無い
		}

		run.Unregistered = len(conflicts)
		run.FinishedAt = uc.now()
		runID, err := uc.syncRepo.SaveRun(ctx, run)
		if err != nil {
			return err
		}
		run.ID = runID

		var reviewIDs []int64
		if !input.DryRun {
			if reviewIDs, err = uc.saveReviews(ctx, plan, input.Year, runID, now); err != nil {
				return err
			}
			if err := uc.syncRepo.MarkReviewsApplied(ctx, plan.AppliedReviewIDs, runID, now); err != nil {
				return err
			}
		}

		unregistered := make(map[int64]int)
		for _, c := range conflicts {
			unregistered[c.CourseID]++
		}
		return uc.syncRepo.SaveChanges(ctx, runID, buildChanges(plan, registered, unregistered, createdIDs, reviewIDs))
	}); err != nil {
		return nil, err
	}

	if !input.DryRun {
		uc.notifyUnregistered(ctx, plan, conflicts)
	}
	return run, nil
}

// applyCourses は計画どおりに授業を書き換え、新しく作った授業の ID を計画の順で返す。
func (uc *SyncCoursesInteractor) applyCourses(ctx context.Context, plan *SyncPlan, now time.Time) ([]int64, error) {
	// 使い回しと判断した旧授業は、同じ dedup_key で新しい授業を作る前に退避する。
	var retireIDs, discontinueIDs []int64
	for _, d := range plan.Discontinues {
		if d.Retire {
			retireIDs = append(retireIDs, d.Course.ID)
		} else {
			discontinueIDs = append(discontinueIDs, d.Course.ID)
		}
	}
	if err := uc.syncRepo.RetireCourses(ctx, retireIDs, now); err != nil {
		return nil, err
	}
	if err := uc.syncRepo.DiscontinueCourses(ctx, discontinueIDs, now); err != nil {
		return nil, err
	}

	params := make([]repository.CourseUpdateParam, 0, len(plan.Updates))
	for _, u := range plan.Updates {
		params = append(params, repository.CourseUpdateParam{
			ID:          u.Before.ID,
			RoomID:      u.Before.RoomID,
			Semester:    u.After.Semester,
			DayOfWeek:   u.After.DayOfWeek,
			Period:      u.After.Period,
			CourseName:  u.After.CourseName,
			TeacherName: u.After.TeacherName,
			SourceRef:   u.After.SourceRef,
			SourceName:  u.After.SourceName,
			DedupKey:    u.After.DedupKey,
			Restore:     u.Restore,
			RenameRoom:  u.After.CourseName != u.Before.CourseName,
		})
	}
	if err := uc.syncRepo.UpdateCourses(ctx, params); err != nil {
		return nil, err
	}

	saveParams := make([]repository.SaveCourseParam, 0, len(plan.Creates))
	for _, c := range plan.Creates {
		saveParams = append(saveParams, saveParamOf(c.Input))
	}
	created, err := uc.courseRepo.SaveCoursesWithRooms(ctx, saveParams)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(created))
	for i, c := range created {
		ids[i] = c.ID
	}
	return ids, nil
}

func (uc *SyncCoursesInteractor) saveReviews(ctx context.Context, plan *SyncPlan, year int, runID int64, now time.Time) ([]int64, error) {
	ids := make([]int64, len(plan.Reviews))
	for i, r := range plan.Reviews {
		id, err := uc.syncRepo.UpsertReview(ctx, &model.CourseSyncReview{
			Year:        year,
			Kind:        r.Kind,
			Fingerprint: r.Fingerprint,
			Status:      model.CourseSyncReviewPending,
			Message:     r.Message,
			Existing:    existingSnapshots(r.Existing),
			Proposed:    proposedSnapshots(r.Proposed),
			FirstRunID:  &runID,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

// notifyUnregistered は時間割から外した人に知らせる。同期そのものはもう確定している
// ので、通知に失敗しても同期を失敗にはしない（ログに残す）。
func (uc *SyncCoursesInteractor) notifyUnregistered(ctx context.Context, plan *SyncPlan, conflicts []repository.SlotConflict) {
	if len(conflicts) == 0 {
		return
	}
	updates := make(map[int64]PlannedUpdate, len(plan.Updates))
	for _, u := range plan.Updates {
		updates[u.Before.ID] = u
	}
	target := notification.TargetCourse
	params := make([]notification.PublishParams, 0, len(conflicts))
	for _, c := range conflicts {
		u, ok := updates[c.CourseID]
		if !ok {
			continue
		}
		courseID := c.CourseID
		params = append(params, notification.PublishParams{
			UserID:     c.UserID,
			Type:       notification.TypeTimetableRemoved,
			TargetType: &target,
			TargetID:   &courseID,
			Message: fmt.Sprintf("「%s」のコマが%sから%sに変わり、同じコマに登録している「%s」と重なったため、時間割から外しました。",
				u.After.CourseName, slotText(u.Before.Semester, u.Before.DayOfWeek, u.Before.Period),
				slotText(u.After.Semester, u.After.DayOfWeek, u.After.Period), c.KeptCourseName),
		})
	}
	if err := uc.notifier.PublishBatch(ctx, params); err != nil {
		logger.Log.Error().Err(err).Int("count", len(params)).Msg("failed to notify users whose timetable entries were removed by the course sync")
	}
}

func slotText(semester, day string, period int) string {
	return fmt.Sprintf("%s%s曜%d限", semester, day, period)
}

// movedCourseIDs はコマか学期が変わった授業（時間割の重なりが新しく起き得る授業）。
func movedCourseIDs(plan *SyncPlan) []int64 {
	var ids []int64
	for _, u := range plan.Updates {
		if u.SlotChanged || u.Before.Semester != u.After.Semester {
			ids = append(ids, u.Before.ID)
		}
	}
	return ids
}

// affectedCourseIDs は変更の記録に登録人数を載せる授業。
func affectedCourseIDs(plan *SyncPlan) []int64 {
	var ids []int64
	for _, u := range plan.Updates {
		ids = append(ids, u.Before.ID)
	}
	for _, d := range plan.Discontinues {
		ids = append(ids, d.Course.ID)
	}
	for _, r := range plan.Reviews {
		for _, c := range r.Existing {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// buildChanges は計画を変更の記録にする。createdIDs / reviewIDs は本実行で採番された
// ID（ドライランでは nil）。
func buildChanges(plan *SyncPlan, registered, unregistered map[int64]int, createdIDs, reviewIDs []int64) []*model.CourseSyncChange {
	changes := make([]*model.CourseSyncChange, 0, len(plan.Creates)+len(plan.Updates)+len(plan.Discontinues)+len(plan.Reviews))
	for i, c := range plan.Creates {
		after := snapshotOfInput(c.Input)
		change := &model.CourseSyncChange{
			Kind:        model.CourseSyncChangeCreated,
			CourseName:  c.Input.CourseName,
			TeacherName: c.Input.TeacherName,
			Detail:      orDefault(c.Detail, "シラバスに新しく掲載"),
			After:       &after,
		}
		if createdIDs != nil {
			id := createdIDs[i]
			change.CourseID = &id
			change.After.CourseID = id
		}
		changes = append(changes, change)
	}
	for _, u := range plan.Updates {
		if u.Silent {
			continue
		}
		before := model.SnapshotOf(u.Before)
		after := model.CourseSnapshot{
			CourseID:    u.Before.ID,
			SourceRef:   u.After.SourceRef,
			Semester:    u.After.Semester,
			DayOfWeek:   u.After.DayOfWeek,
			Period:      u.After.Period,
			CourseName:  u.After.CourseName,
			TeacherName: u.After.TeacherName,
		}
		kind := model.CourseSyncChangeUpdated
		if u.Restore {
			kind = model.CourseSyncChangeRestored
		}
		id := u.Before.ID
		changes = append(changes, &model.CourseSyncChange{
			Kind:              kind,
			CourseID:          &id,
			CourseName:        u.After.CourseName,
			TeacherName:       u.After.TeacherName,
			Detail:            u.Detail,
			Before:            &before,
			After:             &after,
			RegisteredCount:   registered[id],
			UnregisteredCount: unregistered[id],
		})
	}
	for _, d := range plan.Discontinues {
		before := model.SnapshotOf(d.Course)
		id := d.Course.ID
		changes = append(changes, &model.CourseSyncChange{
			Kind:            model.CourseSyncChangeDiscontinued,
			CourseID:        &id,
			CourseName:      d.Course.CourseName,
			TeacherName:     d.Course.TeacherName,
			Detail:          d.Detail,
			Before:          &before,
			RegisteredCount: registered[id],
		})
	}
	for i, r := range plan.Reviews {
		change := &model.CourseSyncChange{
			Kind:   model.CourseSyncChangeReview,
			Detail: r.Message,
		}
		if len(r.Existing) > 0 {
			before := model.SnapshotOf(r.Existing[0])
			change.Before = &before
			change.CourseName = r.Existing[0].CourseName
			change.TeacherName = r.Existing[0].TeacherName
			id := r.Existing[0].ID
			change.CourseID = &id
		}
		if len(r.Proposed) > 0 {
			after := snapshotOfInput(r.Proposed[0])
			change.After = &after
			if change.CourseName == "" {
				change.CourseName = r.Proposed[0].CourseName
				change.TeacherName = r.Proposed[0].TeacherName
			}
		}
		for _, c := range r.Existing {
			change.RegisteredCount += registered[c.ID]
		}
		if reviewIDs != nil {
			id := reviewIDs[i]
			change.ReviewID = &id
		}
		changes = append(changes, change)
	}
	return changes
}

func saveParamOf(in ScrapedCourseInput) repository.SaveCourseParam {
	return repository.SaveCourseParam{
		DayOfWeek:   in.DayOfWeek,
		Period:      in.Period,
		TeacherName: in.TeacherName,
		CourseName:  in.CourseName,
		Year:        in.Year,
		Semester:    in.Semester,
		DedupKey:    in.DedupKey,
		Source:      model.CourseSourceSenshu,
		SourceRef:   in.SourceRef,
		SourceName:  in.SourceName,
	}
}

func snapshotOfInput(in ScrapedCourseInput) model.CourseSnapshot {
	return model.CourseSnapshot{
		SourceRef:   in.SourceRef,
		Semester:    in.Semester,
		DayOfWeek:   in.DayOfWeek,
		Period:      in.Period,
		CourseName:  in.CourseName,
		TeacherName: in.TeacherName,
	}
}

func existingSnapshots(cs []*model.Course) []model.CourseSnapshot {
	out := make([]model.CourseSnapshot, 0, len(cs))
	for _, c := range cs {
		out = append(out, model.SnapshotOf(c))
	}
	return out
}

func proposedSnapshots(ins []ScrapedCourseInput) []model.CourseSnapshot {
	out := make([]model.CourseSnapshot, 0, len(ins))
	for _, in := range ins {
		out = append(out, snapshotOfInput(in))
	}
	return out
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
