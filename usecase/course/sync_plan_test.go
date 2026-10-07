package course

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// ■ シラバス同期の照合規則のテスト
//
// DB の授業（existing）とシラバスの内容（scraped）を並べ、計画がどうなるかを見る。
// 廃止の割合ガード（10%）に掛からないよう、変化させない授業を filler で20件足してある。

const testYear = 2026

func dbCourse(id int64, ref, day string, period int, name, teacher string) *model.Course {
	return &model.Course{
		ID: id, RoomID: id + 1000, Year: testYear, Semester: model.SemesterFirst,
		DayOfWeek: day, Period: period, CourseName: name, TeacherName: teacher,
		Source: model.CourseSourceSenshu, SourceRef: ref, SourceName: name,
		DedupKey: dedupKeyOf(ref, day, period),
	}
}

func scrapedCourse(ref, day string, period int, name, teacher string) ScrapedCourseInput {
	return ScrapedCourseInput{
		Year: testYear, Semester: model.SemesterFirst, DayOfWeek: day, Period: period,
		CourseName: name, SourceName: name, TeacherName: teacher, SourceRef: ref,
		DedupKey: dedupKeyOf(ref, day, period),
	}
}

func dedupKeyOf(ref, day string, period int) string {
	return fmt.Sprintf("senshu:%d:%s:%s:%s:%d", testYear, model.SemesterFirst, ref, day, period)
}

// filler は変化しない授業を n 件ずつ DB とシラバスの両方に作る。
func filler(n int) ([]*model.Course, []ScrapedCourseInput) {
	var olds []*model.Course
	var news []ScrapedCourseInput
	for i := 0; i < n; i++ {
		ref := fmt.Sprintf("F%03d", i)
		olds = append(olds, dbCourse(int64(9000+i), ref, "土", 1, "固定"+ref, "固定先生"))
		news = append(news, scrapedCourse(ref, "土", 1, "固定"+ref, "固定先生"))
	}
	return olds, news
}

func complete(n int) FetchCompleteness {
	return FetchCompleteness{SiteTotal: n, ListedRows: n}
}

func planWith(t *testing.T, olds []*model.Course, news []ScrapedCourseInput, reviews ...*model.CourseSyncReview) *SyncPlan {
	t.Helper()
	fo, fn := filler(20)
	return PlanCourseSync(testYear, append(olds, fo...), append(news, fn...), reviews, complete(len(news)+20))
}

func TestPlan_NothingChangedIsUnchanged(t *testing.T) {
	plan := planWith(t,
		[]*model.Course{dbCourse(1, "A", "水", 5, "経済学入門", "田中")},
		[]ScrapedCourseInput{scrapedCourse("A", "水", 5, "経済学入門", "田中")})
	if len(plan.Updates)+len(plan.Creates)+len(plan.Discontinues)+len(plan.Reviews) != 0 {
		t.Fatalf("plan = %+v, want no changes", plan)
	}
	if plan.Unchanged != 21 {
		t.Errorf("unchanged = %d, want 21", plan.Unchanged)
	}
}

// 水5 → 水4 は同じ授業のコマ変更として更新する（新規作成も廃止もしない）。
func TestPlan_SlotMoveUpdatesTheSameCourse(t *testing.T) {
	plan := planWith(t,
		[]*model.Course{dbCourse(1, "A", "水", 5, "経済学入門", "田中")},
		[]ScrapedCourseInput{scrapedCourse("A", "水", 4, "経済学入門", "田中")})

	if len(plan.Creates) != 0 || len(plan.Discontinues) != 0 {
		t.Fatalf("creates = %d, discontinues = %d; a slot move must not recreate the course", len(plan.Creates), len(plan.Discontinues))
	}
	if len(plan.Updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(plan.Updates))
	}
	u := plan.Updates[0]
	if u.Before.ID != 1 || u.After.DayOfWeek != "水" || u.After.Period != 4 || !u.SlotChanged || u.Silent {
		t.Errorf("update = %+v, want course 1 moved to 水4", u)
	}
	if u.After.DedupKey != dedupKeyOf("A", "水", 4) {
		t.Errorf("dedup key = %q, want it to follow the new slot", u.After.DedupKey)
	}
	if !strings.Contains(u.Detail, "コマ: 水5 → 水4") {
		t.Errorf("detail = %q", u.Detail)
	}
}

func TestPlan_TeacherChangeAloneIsAnUpdate(t *testing.T) {
	plan := planWith(t,
		[]*model.Course{dbCourse(1, "A", "月", 1, "統計学基礎", "山田")},
		[]ScrapedCourseInput{scrapedCourse("A", "月", 1, "統計学基礎", "佐藤")})
	if len(plan.Updates) != 1 || len(plan.Reviews) != 0 {
		t.Fatalf("updates = %d, reviews = %d; want 1 / 0", len(plan.Updates), len(plan.Reviews))
	}
	if got := plan.Updates[0].After.TeacherName; got != "佐藤" {
		t.Errorf("teacher = %q, want 佐藤", got)
	}
}

// 授業名と教員名が両方変わったら使い回しを疑い、何も変えずに確認へ回す。
func TestPlan_NameAndTeacherBothChangedGoesToReview(t *testing.T) {
	olds := []*model.Course{dbCourse(1, "A", "月", 1, "統計学基礎", "山田")}
	news := []ScrapedCourseInput{scrapedCourse("A", "月", 1, "日本文化論", "鈴木")}
	plan := planWith(t, olds, news)

	if len(plan.Updates)+len(plan.Creates)+len(plan.Discontinues) != 0 {
		t.Fatalf("plan = %+v, want the course left alone while under review", plan)
	}
	if len(plan.Reviews) != 1 || plan.Reviews[0].Kind != model.CourseSyncReviewCodeReused {
		t.Fatalf("reviews = %+v, want one CODE_REUSED", plan.Reviews)
	}

	// 同じ状況なら同じ fingerprint（再実行で積み増さない）。
	again := planWith(t, olds, news)
	if again.Reviews[0].Fingerprint != plan.Reviews[0].Fingerprint {
		t.Error("fingerprint is not stable across runs")
	}
}

func TestPlan_CodeReusedDecisions(t *testing.T) {
	olds := []*model.Course{dbCourse(1, "A", "月", 1, "統計学基礎", "山田")}
	news := []ScrapedCourseInput{scrapedCourse("A", "月", 1, "日本文化論", "鈴木")}
	fp := planWith(t, olds, news).Reviews[0].Fingerprint

	t.Run("SAME updates the existing course", func(t *testing.T) {
		plan := planWith(t, olds, news, &model.CourseSyncReview{ID: 77, Fingerprint: fp, Status: model.CourseSyncReviewSame})
		if len(plan.Updates) != 1 || plan.Updates[0].After.CourseName != "日本文化論" {
			t.Fatalf("updates = %+v, want course 1 renamed", plan.Updates)
		}
		if len(plan.Reviews) != 0 || len(plan.AppliedReviewIDs) != 1 || plan.AppliedReviewIDs[0] != 77 {
			t.Errorf("reviews = %d, applied = %v; want the decision applied", len(plan.Reviews), plan.AppliedReviewIDs)
		}
	})
	t.Run("DIFFERENT retires the old course and creates a new one", func(t *testing.T) {
		plan := planWith(t, olds, news, &model.CourseSyncReview{ID: 77, Fingerprint: fp, Status: model.CourseSyncReviewDifferent})
		if len(plan.Discontinues) != 1 || !plan.Discontinues[0].Retire || plan.Discontinues[0].Course.ID != 1 {
			t.Fatalf("discontinues = %+v, want course 1 retired", plan.Discontinues)
		}
		if len(plan.Creates) != 1 || plan.Creates[0].Input.CourseName != "日本文化論" {
			t.Fatalf("creates = %+v, want the new course created", plan.Creates)
		}
	})
	t.Run("IGNORED leaves everything as is", func(t *testing.T) {
		plan := planWith(t, olds, news, &model.CourseSyncReview{ID: 77, Fingerprint: fp, Status: model.CourseSyncReviewIgnored})
		if len(plan.Updates)+len(plan.Creates)+len(plan.Discontinues)+len(plan.Reviews)+len(plan.AppliedReviewIDs) != 0 {
			t.Fatalf("plan = %+v, want nothing", plan)
		}
	})
}

func TestPlan_VanishedCourseIsDiscontinued(t *testing.T) {
	plan := planWith(t,
		[]*model.Course{dbCourse(1, "A", "火", 2, "日本文化論", "鈴木")},
		nil)
	if len(plan.Discontinues) != 1 || plan.Discontinues[0].Course.ID != 1 || plan.Discontinues[0].Retire {
		t.Fatalf("discontinues = %+v, want course 1 discontinued", plan.Discontinues)
	}
	if plan.DiscontinueSkippedReason != "" {
		t.Errorf("skipped reason = %q, want none", plan.DiscontinueSkippedReason)
	}
}

// 既に廃止済みで今回も無い授業は、改めて廃止しない（廃止日時を動かさない）。
func TestPlan_AlreadyDiscontinuedStaysQuiet(t *testing.T) {
	gone := dbCourse(1, "A", "火", 2, "日本文化論", "鈴木")
	at := time.Unix(1, 0)
	gone.DiscontinuedAt = &at
	plan := planWith(t, []*model.Course{gone}, nil)
	if len(plan.Discontinues) != 0 {
		t.Fatalf("discontinues = %+v, want none", plan.Discontinues)
	}
}

func TestPlan_DiscontinuedCourseThatReturnsIsRestored(t *testing.T) {
	gone := dbCourse(1, "A", "火", 2, "日本文化論", "鈴木")
	at := time.Unix(1, 0)
	gone.DiscontinuedAt = &at
	plan := planWith(t, []*model.Course{gone}, []ScrapedCourseInput{scrapedCourse("A", "火", 2, "日本文化論", "鈴木")})
	if len(plan.Updates) != 1 || !plan.Updates[0].Restore || plan.Updates[0].Silent {
		t.Fatalf("updates = %+v, want course 1 restored", plan.Updates)
	}
}

func TestPlan_DiscontinueGuard(t *testing.T) {
	t.Run("an incomplete listing discontinues nothing", func(t *testing.T) {
		fo, fn := filler(20)
		olds := append([]*model.Course{dbCourse(1, "A", "火", 2, "日本文化論", "鈴木")}, fo...)
		plan := PlanCourseSync(testYear, olds, fn, nil, FetchCompleteness{SiteTotal: 25, ListedRows: 20})
		if len(plan.Discontinues) != 0 || plan.DiscontinueSkippedReason == "" {
			t.Fatalf("discontinues = %d, reason = %q; want none and a reason", len(plan.Discontinues), plan.DiscontinueSkippedReason)
		}
	})
	t.Run("unreadable course codes discontinue nothing", func(t *testing.T) {
		fo, fn := filler(20)
		olds := append([]*model.Course{dbCourse(1, "A", "火", 2, "日本文化論", "鈴木")}, fo...)
		plan := PlanCourseSync(testYear, olds, fn, nil, FetchCompleteness{SiteTotal: 21, ListedRows: 21, UnidentifiedRows: 1})
		if len(plan.Discontinues) != 0 || plan.DiscontinueSkippedReason == "" {
			t.Fatalf("discontinues = %d, reason = %q; want none and a reason", len(plan.Discontinues), plan.DiscontinueSkippedReason)
		}
	})
	t.Run("more than 10% vanishing discontinues nothing", func(t *testing.T) {
		fo, fn := filler(20)
		// 20件中3件が消える = 15%。
		plan := PlanCourseSync(testYear, fo, fn[3:], nil, complete(17))
		if len(plan.Discontinues) != 0 || plan.DiscontinueSkippedReason == "" {
			t.Fatalf("discontinues = %d, reason = %q; want none and a reason", len(plan.Discontinues), plan.DiscontinueSkippedReason)
		}
	})
	t.Run("exactly 10% is allowed", func(t *testing.T) {
		fo, fn := filler(20)
		plan := PlanCourseSync(testYear, fo, fn[2:], nil, complete(18))
		if len(plan.Discontinues) != 2 || plan.DiscontinueSkippedReason != "" {
			t.Fatalf("discontinues = %d, reason = %q; want 2 and no reason", len(plan.Discontinues), plan.DiscontinueSkippedReason)
		}
	})
}

// 講義コードが消え、同じ授業名・教員名で新しい講義コードが現れたら、
// 新規作成も廃止もせずに確認へ回す。
func TestPlan_ReissuedCodeGoesToReview(t *testing.T) {
	olds := []*model.Course{dbCourse(1, "A", "木", 3, "マクロ経済学", "高橋")}
	news := []ScrapedCourseInput{scrapedCourse("B", "木", 3, "マクロ経済学", "高橋")}
	plan := planWith(t, olds, news)
	if len(plan.Creates)+len(plan.Discontinues)+len(plan.Updates) != 0 {
		t.Fatalf("plan = %+v, want nothing changed while under review", plan)
	}
	if len(plan.Reviews) != 1 || plan.Reviews[0].Kind != model.CourseSyncReviewCodeReissued {
		t.Fatalf("reviews = %+v, want one CODE_REISSUED", plan.Reviews)
	}

	same := planWith(t, olds, news, &model.CourseSyncReview{ID: 5, Fingerprint: plan.Reviews[0].Fingerprint, Status: model.CourseSyncReviewSame})
	if len(same.Updates) != 1 || same.Updates[0].Before.ID != 1 || same.Updates[0].After.SourceRef != "B" {
		t.Fatalf("updates = %+v, want course 1 re-pointed at code B", same.Updates)
	}
	if len(same.Creates)+len(same.Discontinues) != 0 {
		t.Errorf("creates = %d, discontinues = %d; want none", len(same.Creates), len(same.Discontinues))
	}

	different := planWith(t, olds, news, &model.CourseSyncReview{ID: 5, Fingerprint: plan.Reviews[0].Fingerprint, Status: model.CourseSyncReviewDifferent})
	if len(different.Creates) != 1 || len(different.Discontinues) != 1 {
		t.Errorf("creates = %d, discontinues = %d; want 1 / 1", len(different.Creates), len(different.Discontinues))
	}
}

// 同じ授業名・教員名の候補が複数あると1対1に決まらないので、確認には回さず
// 新規作成と廃止として扱う。
func TestPlan_AmbiguousReissueIsTreatedAsNewAndGone(t *testing.T) {
	olds := []*model.Course{
		dbCourse(1, "A", "木", 3, "英語", "高橋"),
		dbCourse(2, "B", "金", 3, "英語", "高橋"),
	}
	news := []ScrapedCourseInput{scrapedCourse("C", "木", 3, "英語", "高橋")}
	plan := planWith(t, olds, news)
	if len(plan.Reviews) != 0 {
		t.Fatalf("reviews = %+v, want none", plan.Reviews)
	}
	if len(plan.Creates) != 1 || len(plan.Discontinues) != 2 {
		t.Errorf("creates = %d, discontinues = %d; want 1 / 2", len(plan.Creates), len(plan.Discontinues))
	}
}

func TestPlan_MultiSlotCourses(t *testing.T) {
	t.Run("one of two slots moves", func(t *testing.T) {
		plan := planWith(t,
			[]*model.Course{dbCourse(1, "A", "月", 1, "体育", "伊藤"), dbCourse(2, "A", "水", 1, "体育", "伊藤")},
			[]ScrapedCourseInput{scrapedCourse("A", "月", 1, "体育", "伊藤"), scrapedCourse("A", "木", 1, "体育", "伊藤")})
		if len(plan.Updates) != 1 || plan.Updates[0].Before.ID != 2 || plan.Updates[0].After.DayOfWeek != "木" {
			t.Fatalf("updates = %+v, want course 2 moved to 木1", plan.Updates)
		}
	})
	t.Run("a slot is added", func(t *testing.T) {
		plan := planWith(t,
			[]*model.Course{dbCourse(1, "A", "月", 1, "体育", "伊藤")},
			[]ScrapedCourseInput{scrapedCourse("A", "月", 1, "体育", "伊藤"), scrapedCourse("A", "木", 1, "体育", "伊藤")})
		if len(plan.Creates) != 1 || plan.Creates[0].Input.DayOfWeek != "木" || len(plan.Updates) != 0 {
			t.Fatalf("creates = %+v, updates = %+v; want 木1 created", plan.Creates, plan.Updates)
		}
	})
	t.Run("a slot is dropped", func(t *testing.T) {
		plan := planWith(t,
			[]*model.Course{dbCourse(1, "A", "月", 1, "体育", "伊藤"), dbCourse(2, "A", "水", 1, "体育", "伊藤")},
			[]ScrapedCourseInput{scrapedCourse("A", "月", 1, "体育", "伊藤")})
		if len(plan.Discontinues) != 1 || plan.Discontinues[0].Course.ID != 2 {
			t.Fatalf("discontinues = %+v, want course 2", plan.Discontinues)
		}
	})
	t.Run("an unclear reshuffle goes to review", func(t *testing.T) {
		olds := []*model.Course{dbCourse(1, "A", "月", 1, "体育", "伊藤"), dbCourse(2, "A", "水", 1, "体育", "伊藤")}
		news := []ScrapedCourseInput{scrapedCourse("A", "火", 2, "体育", "伊藤"), scrapedCourse("A", "木", 2, "体育", "伊藤"), scrapedCourse("A", "金", 2, "体育", "伊藤")}
		plan := planWith(t, olds, news)
		if len(plan.Reviews) != 1 || plan.Reviews[0].Kind != model.CourseSyncReviewSlotAmbiguous {
			t.Fatalf("reviews = %+v, want one SLOT_AMBIGUOUS", plan.Reviews)
		}
		if len(plan.Updates)+len(plan.Creates)+len(plan.Discontinues) != 0 {
			t.Errorf("plan = %+v, want nothing changed while under review", plan)
		}

		different := planWith(t, olds, news, &model.CourseSyncReview{ID: 9, Fingerprint: plan.Reviews[0].Fingerprint, Status: model.CourseSyncReviewDifferent})
		if len(different.Discontinues) != 2 || len(different.Creates) != 3 {
			t.Errorf("discontinues = %d, creates = %d; want 2 / 3", len(different.Discontinues), len(different.Creates))
		}
	})
}

// 同期前から在る行（source_name 不明）に校舎サフィックスが付いていても、
// 名前の変更とはみなさず、表示名を保ったまま source_name だけを補う。
func TestPlan_KeepsCampusSuffixOfLegacyRows(t *testing.T) {
	legacy := dbCourse(1, "A", "月", 1, "情報科教育法１（生田・3年）", "中村")
	legacy.SourceName = ""
	plan := planWith(t, []*model.Course{legacy}, []ScrapedCourseInput{scrapedCourse("A", "月", 1, "情報科教育法１", "中村")})

	if len(plan.Updates) != 1 {
		t.Fatalf("updates = %d, want 1 (the silent source_name backfill)", len(plan.Updates))
	}
	u := plan.Updates[0]
	if !u.Silent {
		t.Errorf("update = %+v, want it silent (nothing visible changed)", u)
	}
	if u.After.CourseName != "情報科教育法１（生田・3年）" || u.After.SourceName != "情報科教育法１" {
		t.Errorf("name = %q / source name = %q", u.After.CourseName, u.After.SourceName)
	}
}

// 授業名が変わったときは、付いていたサフィックスを新しい名前に付け直す。
func TestPlan_RenameKeepsTheSuffix(t *testing.T) {
	c := dbCourse(1, "A", "月", 1, "情報科教育法１（生田・3年）", "中村")
	c.SourceName = "情報科教育法１"
	plan := planWith(t, []*model.Course{c}, []ScrapedCourseInput{scrapedCourse("A", "月", 1, "情報科教育法Ⅰ", "中村")})
	if len(plan.Updates) != 1 || plan.Updates[0].After.CourseName != "情報科教育法Ⅰ（生田・3年）" {
		t.Fatalf("updates = %+v, want the suffix carried over", plan.Updates)
	}
}

// スクレイパーが今回確かめた名前（Disambiguated）はそのまま使う（サフィックスが外れる場合も）。
func TestPlan_DisambiguatedNameWins(t *testing.T) {
	c := dbCourse(1, "A", "月", 1, "情報科教育法１（生田・3年）", "中村")
	c.SourceName = "情報科教育法１"
	in := scrapedCourse("A", "月", 1, "情報科教育法１", "中村")
	in.Disambiguated = true
	plan := planWith(t, []*model.Course{c}, []ScrapedCourseInput{in})
	if len(plan.Updates) != 1 || plan.Updates[0].After.CourseName != "情報科教育法１" || plan.Updates[0].Silent {
		t.Fatalf("updates = %+v, want the suffix dropped as a visible update", plan.Updates)
	}
}

func TestPlan_NewCourseIsCreated(t *testing.T) {
	plan := planWith(t, nil, []ScrapedCourseInput{scrapedCourse("Z", "金", 5, "新設科目", "小林")})
	if len(plan.Creates) != 1 || plan.Creates[0].Input.SourceRef != "Z" {
		t.Fatalf("creates = %+v, want the new course", plan.Creates)
	}
}
