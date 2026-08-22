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
