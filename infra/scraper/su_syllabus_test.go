package scraper

import (
	"os"
	"testing"

	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
)

// The fixture is a real search-results page captured from the target site (200
// courses, page size switched to the max via navigateKougiList), used to pin the
// scraping regexes against actual markup instead of a hand-written approximation
// that could drift from what the site really sends.
func loadFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/senshu_search_results_page.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(b)
}

func TestExtractTotalCount(t *testing.T) {
	total, err := extractTotalCount(loadFixture(t))
	if err != nil {
		t.Fatalf("extractTotalCount: %v", err)
	}
	if total != 9259 {
		t.Fatalf("total = %d, want 9259", total)
	}
}

func TestExtractTimestamp(t *testing.T) {
	timestamp, err := extractTimestamp(loadFixture(t))
	if err != nil {
		t.Fatalf("extractTimestamp: %v", err)
	}
	if timestamp == "" {
		t.Fatal("timestamp is empty")
	}
}

func TestParseRows(t *testing.T) {
	courses, skipped := parseRows(loadFixture(t), 2026)

	if len(courses) == 0 {
		t.Fatal("parsed 0 courses from a fixture known to contain listings")
	}
	if skipped == 0 {
		t.Fatal("expected at least one skipped row (定時外 offerings are present in the fixture)")
	}

	want := courseusecase.ScrapedCourseInput{
		DayOfWeek:   "木",
		Period:      3,
		TeacherName: "金　鐘勲",
		CourseName:  "アカウンティングコミュニケーション",
		Year:        2026,
		Semester:    "前期",
		DedupKey:    "senshu:2026:前期:26070:木:3",
	}
	if courses[0] != want {
		t.Fatalf("first course = %+v, want %+v", courses[0], want)
	}

	for _, c := range courses {
		if c.DayOfWeek == "" || c.Period == 0 || c.CourseName == "" || c.TeacherName == "" {
			t.Fatalf("incomplete course parsed: %+v", c)
		}
		if c.Year != 2026 || (c.Semester != "前期" && c.Semester != "後期") {
			t.Fatalf("unexpected year/semester: %+v", c)
		}
	}
}

// The captured fixture happens not to contain any 通年 (full-year) rows, so this
// pins the format observed live on the site (same shape as 前期/後期: "通年　X曜日　N時限")
// against a synthetic row built from the fixture's own row markup.
func TestParseRows_FullYearCourse(t *testing.T) {
	row := `<tr class="column_odd" style="vertical-align:middle" >
              <td style="text-align:center">1</td>
              <td>
                <a href="/syllsenshu/slbssbdr.do?value(risyunen)=2026&value(semekikn)=1&value(kougicd)=99999&value(crclumcd)=" >通年テスト科目</a>
            </td>
              <td>通年　月曜日　1時限</td>
              <td>テスト　太郎</td>
          </tr>`

	courses, skipped := parseRows(row, 2026)
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0 (通年 rows with a single day/period slot must not be skipped)", skipped)
	}
	if len(courses) != 1 {
		t.Fatalf("len(courses) = %d, want 1", len(courses))
	}

	want := courseusecase.ScrapedCourseInput{
		DayOfWeek:   "月",
		Period:      1,
		TeacherName: "テスト　太郎",
		CourseName:  "通年テスト科目",
		Year:        2026,
		Semester:    "通年",
		DedupKey:    "senshu:2026:通年:99999:月:1",
	}
	if courses[0] != want {
		t.Fatalf("course = %+v, want %+v", courses[0], want)
	}
}

// A co-taught course's teacher cell contains an embedded <br/> between the two
// teachers' names, which used to make rowRe fail to match the row at all - not
// merely fail to parse it - so the course vanished without even being counted in
// skipped. This pins the fix: the row must still match, and the two names must be
// joined into a single TeacherName rather than truncated at the first name.
func TestParseRows_CoTaughtCourse(t *testing.T) {
	row := `<tr class="column_odd" style="vertical-align:middle" >
              <td style="text-align:center">1</td>
              <td>
                <a href="/syllsenshu/slbssbdr.do?value(risyunen)=2026&value(semekikn)=1&value(kougicd)=32212&value(crclumcd)=" >コンピュータサイエンス演習１</a>
            </td>
              <td>前期　水曜日　4時限</td>
              <td>安藤　映<br/>重中　秀介</td>
          </tr>`

	courses, skipped := parseRows(row, 2026)
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(courses) != 1 {
		t.Fatalf("len(courses) = %d, want 1", len(courses))
	}

	want := courseusecase.ScrapedCourseInput{
		DayOfWeek:   "水",
		Period:      4,
		TeacherName: "安藤　映、重中　秀介",
		CourseName:  "コンピュータサイエンス演習１",
		Year:        2026,
		Semester:    "前期",
		DedupKey:    "senshu:2026:前期:32212:水:4",
	}
	if courses[0] != want {
		t.Fatalf("course = %+v, want %+v", courses[0], want)
	}
}

// A course meeting two slots per week (e.g. consecutive periods) puts both in the
// period cell separated by <br/>. The current model only represents one weekly
// slot per course, so this must still be skipped - but, unlike before the rowRe
// fix, counted in skipped rather than silently vanishing.
func TestParseRows_MultiSlotCourseIsSkipped(t *testing.T) {
	row := `<tr class="column_odd" style="vertical-align:middle" >
              <td style="text-align:center">1</td>
              <td>
                <a href="/syllsenshu/slbssbdr.do?value(risyunen)=2026&value(semekikn)=1&value(kougicd)=88888&value(crclumcd)=" >マルチスロット科目</a>
            </td>
              <td>前期　月曜日　4時限<br/>前期　月曜日　5時限</td>
              <td>テスト　次郎</td>
          </tr>`

	courses, skipped := parseRows(row, 2026)
	if len(courses) != 0 {
		t.Fatalf("len(courses) = %d, want 0 (multi-slot courses aren't representable yet)", len(courses))
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
}
