// Package scraper adapts external, HTML-scraped course catalogs into the
// courseusecase.ScrapedCourseInput shape expected by ImportCoursesUseCase. It is
// intentionally kept outside usecase/repository: if the source site changes its
// markup, or a different university's syllabus site needs to be supported, only
// this package needs to change.
package scraper

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	courseusecase "github.com/Cityboypenguin/SPACE-server/usecase/course"
)

// 専修大学Web講義要項（シラバス）. Its search screen (slbssrch.do) guards form
// submission with a session-scoped, single-use "timestamp" token that is reissued
// on every response, so results can only be paged through sequentially, not fetched
// in parallel or resumed from a token captured earlier. The listing itself already
// carries course name / teacher / day / period, so no per-course detail-page fetch
// is needed.
const (
	senshuBaseURL   = "https://syllabus.acc.senshu-u.ac.jp/syllsenshu"
	senshuSearchDo  = senshuBaseURL + "/slbssrch.do"
	senshuUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"

	// pageSize is the largest page size the site's own UI offers (10/20/30/40/50/100/200).
	// Using the max keeps the total number of requests against the university's server low.
	pageSize = 200
)

var (
	timestampRe  = regexp.MustCompile(`name="timestamp"\s+value="([^"]*)"`)
	totalCountRe = regexp.MustCompile(`件表示/([0-9,]+)件中`)
	rowRe        = regexp.MustCompile(`(?s)<tr class="column_(?:odd|even)"[^>]*>\s*<td[^>]*>\s*\d+\s*</td>\s*<td>\s*<a href="([^"]+)"[^>]*>([^<]*)</a>\s*</td>\s*<td>([^<]*)</td>\s*<td>([^<]*)</td>\s*</tr>`)
	kougicdRe    = regexp.MustCompile(`kougicd\)=(\d+)`)
	periodCellRe = regexp.MustCompile(`^(前期|後期)\x{3000}(月|火|水|木|金|土)曜日\x{3000}(\d+)時限$`)
)

// SenshuSyllabusScraper fetches the full course catalog for a given academic year
// from the 専修大学 public syllabus site.
//
// Requests are shelled out to curl rather than sent via net/http: the site only
// offers a classic DHE (finite-field, non-ECDHE) TLS cipher suite, which Go's
// crypto/tls client has never implemented (it supports ECDHE/TLS1.3 key exchange
// only), so a pure-Go client cannot complete the TLS handshake at all. curl, backed
// by the system's OpenSSL/LibreSSL, has no such restriction. This requires curl to
// be present wherever this scraper runs.
type SenshuSyllabusScraper struct {
	cookieJarPath string
	// RequestInterval is the delay between successive page requests. The site has
	// no documented rate limit, so this exists purely to be a polite, low-impact
	// caller rather than to satisfy a known constraint.
	RequestInterval time.Duration
}

func NewSenshuSyllabusScraper() (*SenshuSyllabusScraper, error) {
	f, err := os.CreateTemp("", "senshu-syllabus-cookiejar-*.txt")
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar file: %w", err)
	}
	_ = f.Close()
	return &SenshuSyllabusScraper{
		cookieJarPath:   f.Name(),
		RequestInterval: 500 * time.Millisecond,
	}, nil
}

// Close removes the temporary cookie jar file backing this scraper's session.
func (s *SenshuSyllabusScraper) Close() error {
	return os.Remove(s.cookieJarPath)
}

// FetchCourses retrieves every course offered in the given academic year (both
// semesters), across all departments. skipped counts listing rows that could not
// be mapped to a single weekly day/period slot (e.g. 定時外 or 通年 offerings),
// which the current timetable model does not represent.
func (s *SenshuSyllabusScraper) FetchCourses(ctx context.Context, year int) (courses []courseusecase.ScrapedCourseInput, skipped int, err error) {
	body, err := s.get(ctx, senshuSearchDo)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching search form: %w", err)
	}
	timestamp, err := extractTimestamp(body)
	if err != nil {
		return nil, 0, err
	}

	body, err = s.post(ctx, senshuSearchDo, url.Values{
		"value(methodname)":                {"sylkougi_search"},
		"timestamp":                        {timestamp},
		"value(searchDetailConditionFlag)": {"1"},
		"value(kouginm)":                   {""},
		"value(syokunm)":                   {""},
		"value(keywords)":                  {""},
		"value(coursecd1)":                 {""},
		"value(coursecd2)":                 {""},
		"value(coursecd3)":                 {""},
		"value(nendo)":                     {strconv.Itoa(year)},
		"value(searchKeywordFlg)":          {"1"},
		"value(crclm)":                     {""},
		"value(campuscd)":                  {""},
		"value(kkikancd)":                  {""},
		"buttonName":                       {"searchKougi"},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("submitting search: %w", err)
	}

	// Switch the just-established search session over to the largest page size.
	timestamp, err = extractTimestamp(body)
	if err != nil {
		return nil, 0, err
	}
	body, err = s.post(ctx, senshuSearchDo, url.Values{
		"timestamp":         {timestamp},
		"value(pageCount)":  {""},
		"value(maxCount)":   {strconv.Itoa(pageSize)},
		"maxDispListCount":  {strconv.Itoa(pageSize)},
		"navigateKougiList": {"dummy"},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("switching page size: %w", err)
	}

	total, err := extractTotalCount(body)
	if err != nil {
		return nil, 0, err
	}

	rows, rowsSkipped := parseRows(body, year)
	courses = append(courses, rows...)
	skipped += rowsSkipped

	totalPages := (total + pageSize - 1) / pageSize
	for page := 2; page <= totalPages; page++ {
		if err := sleepCtx(ctx, s.RequestInterval); err != nil {
			return nil, 0, err
		}

		timestamp, err = extractTimestamp(body)
		if err != nil {
			return nil, 0, err
		}
		body, err = s.post(ctx, senshuSearchDo, url.Values{
			"timestamp":         {timestamp},
			"value(pageCount)":  {strconv.Itoa(page)},
			"value(maxCount)":   {strconv.Itoa(pageSize)},
			"navigateKougiList": {"dummy"},
		})
		if err != nil {
			return nil, 0, fmt.Errorf("fetching page %d: %w", page, err)
		}

		rows, rowsSkipped := parseRows(body, year)
		courses = append(courses, rows...)
		skipped += rowsSkipped
	}

	return courses, skipped, nil
}

func (s *SenshuSyllabusScraper) get(ctx context.Context, target string) (string, error) {
	return s.curl(ctx, "-A", senshuUserAgent, target)
}

func (s *SenshuSyllabusScraper) post(ctx context.Context, target string, values url.Values) (string, error) {
	return s.curl(ctx,
		"-A", senshuUserAgent,
		"-H", "Content-Type: application/x-www-form-urlencoded",
		"-H", "Referer: "+senshuSearchDo,
		"--data-binary", values.Encode(),
		"-X", "POST",
		target,
	)
}

// curl runs curl with cookie persistence across calls, a hard timeout, and -f so
// non-2xx responses surface as an error instead of being returned as body text.
func (s *SenshuSyllabusScraper) curl(ctx context.Context, args ...string) (string, error) {
	fullArgs := append([]string{
		"-s", "-S", "-f", "-L",
		"-b", s.cookieJarPath, "-c", s.cookieJarPath,
		"--max-time", "30",
	}, args...)

	cmd := exec.CommandContext(ctx, "curl", fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("curl request failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func extractTimestamp(body string) (string, error) {
	m := timestampRe.FindStringSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("timestamp token not found in response")
	}
	return m[1], nil
}

func extractTotalCount(body string) (int, error) {
	m := totalCountRe.FindStringSubmatch(body)
	if m == nil {
		return 0, fmt.Errorf("result count not found in response")
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		return 0, fmt.Errorf("parsing result count %q: %w", m[1], err)
	}
	return n, nil
}

// parseRows extracts one ScrapedCourseInput per listing row that names a single
// weekly day/period slot. Rows for 定時外 (non-standard time) or 通年 (full-year)
// offerings don't fit that shape and are reported back via the skipped count
// instead of being silently dropped.
func parseRows(body string, year int) (courses []courseusecase.ScrapedCourseInput, skipped int) {
	for _, m := range rowRe.FindAllStringSubmatch(body, -1) {
		href, name, periodCell, teacher := m[1], m[2], m[3], m[4]

		kougicdMatch := kougicdRe.FindStringSubmatch(href)
		if kougicdMatch == nil {
			skipped++
			continue
		}
		kougicd := kougicdMatch[1]

		cell := strings.TrimSpace(html.UnescapeString(periodCell))
		slot := periodCellRe.FindStringSubmatch(cell)
		if slot == nil {
			skipped++
			continue
		}
		semester, dayOfWeek, periodStr := slot[1], slot[2], slot[3]
		period, err := strconv.Atoi(periodStr)
		if err != nil {
			skipped++
			continue
		}

		courses = append(courses, courseusecase.ScrapedCourseInput{
			DayOfWeek:   dayOfWeek,
			Period:      period,
			TeacherName: strings.TrimSpace(html.UnescapeString(teacher)),
			CourseName:  strings.TrimSpace(html.UnescapeString(name)),
			Year:        year,
			Semester:    semester,
			DedupKey:    fmt.Sprintf("senshu:%d:%s:%s:%s:%d", year, semester, kougicd, dayOfWeek, period),
		})
	}
	return courses, skipped
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
