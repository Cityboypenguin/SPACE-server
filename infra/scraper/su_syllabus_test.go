package scraper

import (
	"os"
	"strings"
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
	rows, skipped := parseRows(loadFixture(t), 2026)

	if len(rows) == 0 {
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
	if rows[0].course != want {
		t.Fatalf("first course = %+v, want %+v", rows[0].course, want)
	}
	if rows[0].detailPath == "" {
		t.Fatal("detailPath is empty")
	}

	for _, r := range rows {
		c := r.course
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

	rows, skipped := parseRows(row, 2026)
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0 (通年 rows with a single day/period slot must not be skipped)", skipped)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
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
	if rows[0].course != want {
		t.Fatalf("course = %+v, want %+v", rows[0].course, want)
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

	rows, skipped := parseRows(row, 2026)
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
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
	if rows[0].course != want {
		t.Fatalf("course = %+v, want %+v", rows[0].course, want)
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

	rows, skipped := parseRows(row, 2026)
	if len(rows) != 0 {
		t.Fatalf("len(rows) = %d, want 0 (multi-slot courses aren't representable yet)", len(rows))
	}
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
}

// disambiguateCampuses must recognize a colliding pair even when campus alone
// wouldn't distinguish them (the two rows are cross-listed for different
// departments/years via 配当, not different campuses) - pinned via keyOf directly
// since the surrounding disambiguateCampuses method needs a live HTTP fetch.
func TestKeyOf_IgnoresCourseNameSuffix(t *testing.T) {
	a := courseusecase.ScrapedCourseInput{Year: 2026, Semester: "前期", DayOfWeek: "木", Period: 4, CourseName: "情報科教育法１", TeacherName: "鶴田　利郎"}
	b := a
	if keyOf(a) != keyOf(b) {
		t.Fatal("identical rows must share a disambiguationKey")
	}
	b.CourseName = "情報科教育法２"
	if keyOf(a) == keyOf(b) {
		t.Fatal("different course names must not share a disambiguationKey")
	}
}

func TestAllKnown(t *testing.T) {
	rows := []scrapedRow{
		{course: courseusecase.ScrapedCourseInput{DedupKey: "a"}},
		{course: courseusecase.ScrapedCourseInput{DedupKey: "b"}},
	}

	if allKnown(rows, []int{0, 1}, map[string]bool{"a": true}) {
		t.Fatal("a group with an unknown member must not be reported as all-known")
	}
	if !allKnown(rows, []int{0, 1}, map[string]bool{"a": true, "b": true}) {
		t.Fatal("a group whose every member is in knownDedupKeys must be reported as all-known")
	}
	if allKnown(rows, []int{0, 1}, nil) {
		t.Fatal("a nil knownDedupKeys map must never report a group as all-known")
	}
}

func TestCleanDetailField(t *testing.T) {
	got := cleanDetailField("\n\n\n一部生田／生田&nbsp;\n\n")
	if want := "一部生田／生田"; got != want {
		t.Fatalf("cleanDetailField = %q, want %q", got, want)
	}
}

// Pins campusRe/assignmentRe against the detail page's actual markup structure
// (captured live from two real course pages that otherwise collide).
func TestCampusAndAssignmentRegexes(t *testing.T) {
	body := `
		<td class="label_kougi">開講区分／校舎</td>
		<td class="line_y_label"></td>
		<td class="kougi">



一部神田／神田&nbsp;

		</td>
		<td class="label_kougi">配　当</td>
		<td class="line_y_label"></td>
		<td class="kougi">



マーケ学科３４&nbsp;

		</td>`

	m := campusRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("campusRe did not match")
	}
	if got := cleanDetailField(m[1]); got != "一部神田／神田" {
		t.Fatalf("campus = %q, want %q", got, "一部神田／神田")
	}

	m = assignmentRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("assignmentRe did not match")
	}
	if got := cleanDetailField(m[1]); got != "マーケ学科３４" {
		t.Fatalf("assignment = %q, want %q", got, "マーケ学科３４")
	}
}

func TestAllSameContent_ByteIdenticalPages(t *testing.T) {
	body := `<td class="kougi">同じ内容の授業です&nbsp;</td>`
	if !allSameContent([]string{body, body}) {
		t.Fatal("two byte-identical pages must be treated as the same course")
	}
}

// Pins the real-world case found in production data: two saves of the same
// course whose only difference is the page's own save timestamp and a trivial
// formatting change to an unrelated, otherwise-empty field. These must still
// merge - only the actual syllabus content matters.
func TestAllSameContent_OnlyTimestampAndTrivialFormattingDiffer(t *testing.T) {
	// A realistic detail page runs to a couple thousand characters; padding with a
	// shared filler block keeps the six-character "【主要授業科目】" difference
	// proportionally as small as it is on a real page (see levenshteinRatio).
	filler := strings.Repeat("シラバス本文のテキストです。", 80)
	a := `<td class="kougi">【主要授業科目】<BR><BR>特になし。&nbsp;` + filler + `</td>
		2025-02-18 17:26:15.122`
	b := `<td class="kougi">特になし。&nbsp;` + filler + `</td>
		2025-03-25 17:06:00.203`
	if !allSameContent([]string{a, b}) {
		t.Fatal("pages differing only by save timestamp and tag-only formatting must be treated as the same course")
	}
}

// Pins the other real-world case: two rows sharing name/teacher/slot that are
// genuinely different course sections (e.g. different seminar themes) - these
// must never be merged into one course.
func TestAllSameContent_GenuinelyDifferentContent(t *testing.T) {
	a := `<td class="kougi">＜到達目標＞卒論・卒制指導を行う。&nbsp;</td>`
	b := `<td class="kougi">＜到達目標＞映像番組の制作を行う。&nbsp;</td>`
	if allSameContent([]string{a, b}) {
		t.Fatal("pages with genuinely different syllabus content must not be treated as the same course")
	}
}

func TestAllSameContent_SingleBodyIsTriviallySame(t *testing.T) {
	if !allSameContent([]string{"anything"}) {
		t.Fatal("a group of one is trivially \"all the same\"")
	}
}

func TestLevenshteinRatio(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want float64
	}{
		{"identical", "同じ文字列", "同じ文字列", 1},
		{"both empty", "", "", 1},
		{"one empty", "何か", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := levenshteinRatio(c.a, c.b); got != c.want {
				t.Fatalf("levenshteinRatio(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}

	if got := levenshteinRatio("あいうえお", "あいうえか"); got != 0.8 {
		t.Fatalf("one-character substitution out of five ratio = %v, want 0.8", got)
	}
}
