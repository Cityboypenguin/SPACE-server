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
// in parallel or resumed from a token captured earlier. The listing itself carries
// course name / teacher / day / period, so no per-course detail-page fetch is
// needed for most rows - only disambiguateCampuses fetches individual detail
// pages (slbssbdr.do), and only for the rows that need it.
const (
	senshuOrigin    = "https://syllabus.acc.senshu-u.ac.jp"
	senshuBaseURL   = senshuOrigin + "/syllsenshu"
	senshuSearchDo  = senshuBaseURL + "/slbssrch.do"
	senshuUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"

	// pageSize is the largest page size the site's own UI offers (10/20/30/40/50/100/200).
	// Using the max keeps the total number of requests against the university's server low.
	pageSize = 200
)

var (
	timestampRe  = regexp.MustCompile(`name="timestamp"\s+value="([^"]*)"`)
	totalCountRe = regexp.MustCompile(`件表示/([0-9,]+)件中`)
	// The period and teacher cells' content is captured non-greedily (rather than
	// [^<]*) because either can contain an embedded <br/> - the period cell when a
	// course meets in two slots per week, the teacher cell when co-taught - which
	// would otherwise make the whole row (and thus the course) invisible to
	// FindAllStringSubmatch instead of merely falling through to a skip below.
	rowRe        = regexp.MustCompile(`(?s)<tr class="column_(?:odd|even)"[^>]*>\s*<td[^>]*>\s*\d+\s*</td>\s*<td>\s*<a href="([^"]+)"[^>]*>([^<]*)</a>\s*</td>\s*<td>(.*?)</td>\s*<td>(.*?)</td>\s*</tr>`)
	kougicdRe    = regexp.MustCompile(`kougicd\)=(\d+)`)
	periodCellRe = regexp.MustCompile(`^(前期|後期|通年)\x{3000}(月|火|水|木|金|土)曜日\x{3000}(\d+)時限$`)
	brTagRe      = regexp.MustCompile(`(?i)<br\s*/?>`)
	campusRe     = regexp.MustCompile(`(?s)開講区分／校舎.*?<td class="kougi">\s*(.*?)\s*</td>`)
	// The label is rendered as "配　当" with a full-width space (\x{3000}), which Go's
	// \s (ASCII-only) doesn't match, so it's matched explicitly alongside \s.
	assignmentRe = regexp.MustCompile(`(?s)配[\s\x{3000}]*当.*?<td class="kougi">\s*(.*?)\s*</td>`)
	tagRe        = regexp.MustCompile(`(?s)<[^>]+>`)
	// The site stamps each detail page with its own last-saved time near the
	// bottom, formatted like "2025-03-25 14:39:33.847" - this is the one piece of
	// visible (non-tag) text that differs between two saves of an otherwise
	// identical page, so it's stripped before comparing pages for equality.
	timestampLineRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(\.\d+)?`)
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
// semesters, plus 通年 full-year courses), across all departments. skipped counts
// listing rows that could not be mapped to a single weekly day/period slot (e.g.
// 定時外 non-standard-time offerings), which the current timetable model does not
// represent.
//
// knownDedupKeys is the set of dedup_key values the caller has already imported
// for this year (nil/empty is fine, e.g. for a year's first-ever import). A
// re-scrape of a year that's already fully imported is a very common case (an
// admin re-running the import to pick up a handful of newly-published courses),
// and every course whose dedup_key is already known is guaranteed to be a no-op
// on import regardless of what FetchCourses returns for it - so disambiguation,
// which is the expensive part (an extra page fetch per colliding row), is
// skipped entirely for any colliding group where every member is already known.
//
// onProgress, if non-nil, is called with a running (fetched, total) pair as work
// completes: after every listing page, and then again for every detail-page fetch
// disambiguateCampuses makes afterwards (total grows to include those once the
// listing is known) - a catalog with many name/teacher/slot collisions can spend
// far longer in that second phase than fetching the listing itself, so without
// this the reported progress would look frozen the moment the listing finishes
// even though the run is still working.
func (s *SenshuSyllabusScraper) FetchCourses(ctx context.Context, year int, knownDedupKeys map[string]bool, onProgress func(fetched, total int)) (courses []courseusecase.ScrapedCourseInput, skipped int, err error) {
	var rows []scrapedRow

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

	pageRows, rowsSkipped := parseRows(body, year)
	rows = append(rows, pageRows...)
	skipped += rowsSkipped
	if onProgress != nil {
		onProgress(len(rows), total)
	}

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

		pageRows, rowsSkipped := parseRows(body, year)
		rows = append(rows, pageRows...)
		skipped += rowsSkipped
		if onProgress != nil {
			onProgress(len(rows), total)
		}
	}

	courses, err = s.disambiguateCampuses(ctx, rows, knownDedupKeys, onProgress, len(rows), total)
	if err != nil {
		return nil, 0, err
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

// curlMaxAttempts is how many times curl runs a request before giving up. The
// university's server occasionally times out or drops a connection under no
// fault of the request itself (seen in practice fetching a listing page: "curl:
// (28) Operation timed out after 30001 milliseconds", and separately a course
// detail page: "Connection timed out after 30002 milliseconds"), and a single
// exhausted request currently aborts the entire scrape - discarding every page
// already fetched, since courses are only saved once FetchCourses returns
// successfully in full (see ImportCoursesUseCase). Retrying several times with
// backoff, and giving each attempt more room via curlMaxTime, costs far less than
// that: an admin re-running an aborted multi-thousand-course scrape over one
// flaky blip near the end.
const curlMaxAttempts = 5

// curlMaxTime is curl's own --max-time budget per attempt, i.e. the ceiling on how
// slow a single response is allowed to be before that attempt is abandoned (not a
// total-request-count budget - see curlMaxAttempts for that).
const curlMaxTime = "45"

// curl runs curl with cookie persistence across calls, a hard per-attempt
// timeout (curlMaxTime), -f so non-2xx responses surface as an error instead of
// being returned as body text, and retries (see curlMaxAttempts) with backoff
// before giving up.
func (s *SenshuSyllabusScraper) curl(ctx context.Context, args ...string) (string, error) {
	fullArgs := append([]string{
		"-s", "-S", "-f", "-L",
		"-b", s.cookieJarPath, "-c", s.cookieJarPath,
		"--max-time", curlMaxTime,
	}, args...)

	var lastErr error
	for attempt := 1; attempt <= curlMaxAttempts; attempt++ {
		cmd := exec.CommandContext(ctx, "curl", fullArgs...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			lastErr = fmt.Errorf("curl request failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
			if attempt < curlMaxAttempts {
				if sleepErr := sleepCtx(ctx, time.Duration(attempt)*2*time.Second); sleepErr != nil {
					return "", sleepErr
				}
				continue
			}
			return "", lastErr
		}
		return stdout.String(), nil
	}
	return "", lastErr
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

// scrapedRow pairs a parsed course with the listing row's detail-page path. The
// path is only used by disambiguateCampuses, which fetches it for the (usually
// small) subset of rows that turn out to collide with another one - the listing
// itself doesn't carry enough information to tell those apart on its own.
type scrapedRow struct {
	course     courseusecase.ScrapedCourseInput
	detailPath string
}

// parseRows extracts one scrapedRow per listing row that names a single weekly
// day/period slot, including 通年 (full-year) rows (Semester will be "通年").
// Rows for 定時外 (non-standard time) offerings don't fit that shape and are
// reported back via the skipped count instead of being silently dropped.
func parseRows(body string, year int) (rows []scrapedRow, skipped int) {
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

		// Co-taught courses separate teacher names with <br/> in this cell; join
		// them with a full-width comma instead since TeacherName is a single field.
		teacherName := strings.TrimSpace(html.UnescapeString(brTagRe.ReplaceAllString(teacher, "、")))

		rows = append(rows, scrapedRow{
			course: courseusecase.ScrapedCourseInput{
				DayOfWeek:   dayOfWeek,
				Period:      period,
				TeacherName: teacherName,
				CourseName:  strings.TrimSpace(html.UnescapeString(name)),
				Year:        year,
				Semester:    semester,
				DedupKey:    fmt.Sprintf("senshu:%d:%s:%s:%s:%d", year, semester, kougicd, dayOfWeek, period),
			},
			detailPath: html.UnescapeString(href),
		})
	}
	return rows, skipped
}

// disambiguationKey groups rows that a student browsing the timetable/search UI
// cannot otherwise tell apart: same name, teacher, and weekly slot. The listing
// doesn't carry anything else, but the same class is sometimes listed once per
// campus or once per enrolling department, so two rows can legitimately share a
// key while being genuinely different course offerings.
type disambiguationKey struct {
	year        int
	semester    string
	dayOfWeek   string
	period      int
	courseName  string
	teacherName string
}

func keyOf(c courseusecase.ScrapedCourseInput) disambiguationKey {
	return disambiguationKey{c.Year, c.Semester, c.DayOfWeek, c.Period, c.CourseName, c.TeacherName}
}

// allKnown reports whether every row referenced by idxs already has its
// dedup_key present in knownDedupKeys.
func allKnown(rows []scrapedRow, idxs []int, knownDedupKeys map[string]bool) bool {
	for _, i := range idxs {
		if !knownDedupKeys[rows[i].course.DedupKey] {
			return false
		}
	}
	return true
}

// disambiguateCampuses resolves rows that collide on disambiguationKey - a
// student browsing the timetable/search UI can't otherwise tell them apart - by
// fetching each colliding row's own detail page, which the listing itself doesn't
// carry. Within a colliding group this only pays for a page fetch per row that
// actually collides, not all of them.
//
// A collision is one of two things in practice:
//   - The same class really is listed twice (an administrative duplicate on the
//     site's own end): its detail page content is identical (a save timestamp and
//     inconsequential formatting aside) across every row in the group. Only the
//     first such row is kept; the rest are dropped rather than imported as
//     separate courses/chat rooms for what is really one class.
//   - The rows are genuinely different offerings under the same name/teacher/slot
//     (e.g. once per campus, or once per enrolling department) - their content
//     differs. Each is kept, with a "（校舎・配当）" suffix appended to CourseName
//     (e.g. "情報科教育法１（生田・...）" vs "情報科教育法１（神田・...）") so
//     students can tell which one to register for.
//
// A colliding group is skipped entirely - no page fetches at all - when every
// member's dedup_key is already in knownDedupKeys: re-importing an already-known
// course is always a no-op, so there's nothing disambiguation could change about
// the outcome for that group.
//
// This is the phase most likely to make FetchCourses look stalled if a catalog
// happens to have many name/teacher/slot collisions (routine for cross-listed or
// multi-campus courses): every group needing disambiguation costs one sequential,
// RequestInterval-spaced detail-page fetch per member, which can add up to far
// longer than the listing itself took - so onProgress, baseProcessed and baseTotal
// let the caller fold this phase's fetches into the same running total instead of
// the reported percentage freezing the moment the listing pages are done.
func (s *SenshuSyllabusScraper) disambiguateCampuses(ctx context.Context, rows []scrapedRow, knownDedupKeys map[string]bool, onProgress func(fetched, total int), baseProcessed, baseTotal int) ([]courseusecase.ScrapedCourseInput, error) {
	groups := make(map[disambiguationKey][]int, len(rows))
	for i, r := range rows {
		k := keyOf(r.course)
		groups[k] = append(groups[k], i)
	}

	type disambiguationJob struct {
		idxs []int
	}
	var jobs []disambiguationJob
	extraFetches := 0
	for _, idxs := range groups {
		if len(idxs) < 2 || allKnown(rows, idxs, knownDedupKeys) {
			continue
		}
		jobs = append(jobs, disambiguationJob{idxs: idxs})
		extraFetches += len(idxs)
	}

	combinedTotal := baseTotal + extraFetches
	if onProgress != nil {
		onProgress(baseProcessed, combinedTotal)
	}

	drop := make(map[int]bool)
	fetched := 0
	for _, job := range jobs {
		idxs := job.idxs
		bodies := make([]string, len(idxs))
		for j, i := range idxs {
			if err := sleepCtx(ctx, s.RequestInterval); err != nil {
				return nil, err
			}
			body, err := s.get(ctx, senshuOrigin+rows[i].detailPath)
			if err != nil {
				return nil, fmt.Errorf("fetching detail page %q: %w", rows[i].detailPath, err)
			}
			bodies[j] = body
			fetched++
			if onProgress != nil {
				onProgress(baseProcessed+fetched, combinedTotal)
			}
		}

		if allSameContent(bodies) {
			for _, i := range idxs[1:] {
				drop[i] = true
			}
			continue
		}

		for j, i := range idxs {
			campus, assignment := extractCampusAndAssignment(bodies[j])
			label := strings.TrimSpace(campus + "・" + assignment)
			label = strings.Trim(label, "・")
			if label != "" {
				rows[i].course.CourseName = fmt.Sprintf("%s（%s）", rows[i].course.CourseName, label)
			}
		}
	}

	courses := make([]courseusecase.ScrapedCourseInput, 0, len(rows))
	for i, r := range rows {
		if drop[i] {
			continue
		}
		courses = append(courses, r.course)
	}
	return courses, nil
}

// extractCampusAndAssignment reads a detail page's campus (開講区分／校舎) and
// target department/year (配当) fields, which the listing itself doesn't include.
func extractCampusAndAssignment(body string) (campus, assignment string) {
	if m := campusRe.FindStringSubmatch(body); m != nil {
		campus = cleanDetailField(m[1])
	}
	if m := assignmentRe.FindStringSubmatch(body); m != nil {
		assignment = cleanDetailField(m[1])
	}
	return campus, assignment
}

// cleanDetailField strips the   padding these detail-page cells are rendered
// with (from &nbsp;) and normalizes the result to plain text.
func cleanDetailField(raw string) string {
	unescaped := html.UnescapeString(raw)
	unescaped = strings.ReplaceAll(unescaped, " ", "")
	return strings.TrimSpace(unescaped)
}

// contentSimilarityThreshold is how close two detail pages' normalized content
// must be (1.0 = identical) to be treated as the same course listed twice. It's
// not 1.0 because two saves of the same syllabus can differ by a stray empty
// field's label (seen in production: "【主要授業科目】" appearing in one save and
// not the other, nothing else different) - a few characters out of a page that's
// easily 1-2 thousand. 0.98 comfortably absorbs that while still rejecting pages
// that genuinely describe a different course (differing across whole paragraphs).
const contentSimilarityThreshold = 0.98

// allSameContent reports whether every detail page in bodies is close enough to
// be the same course listed more than once - cosmetic differences (a save
// timestamp, a stray empty field's exact markup) aside - as opposed to genuinely
// different course content sharing the same name/teacher/slot (e.g. two seminar
// sections with different themes, which must not be merged).
func allSameContent(bodies []string) bool {
	if len(bodies) < 2 {
		return true
	}
	first := normalizeContent(bodies[0])
	for _, b := range bodies[1:] {
		if levenshteinRatio(first, normalizeContent(b)) < contentSimilarityThreshold {
			return false
		}
	}
	return true
}

// normalizeContent strips markup, the page's own save timestamp, and all
// whitespace, leaving just the visible text content for a similarity comparison
// between two detail pages.
func normalizeContent(body string) string {
	text := timestampLineRe.ReplaceAllString(body, "")
	text = tagRe.ReplaceAllString(text, "")
	text = html.UnescapeString(text)
	return strings.Join(strings.Fields(text), "")
}

// levenshteinRatio returns how similar a and b are as a ratio in [0,1], where 1
// means identical and 0 means an edit distance equal to the longer string's
// entire length. Detail pages here run to a couple thousand characters at most,
// so the O(len(a)*len(b)) cost is negligible even though this only ever runs on
// the handful of rows that collide within a single scrape.
func levenshteinRatio(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}

	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			curr[j] = min
		}
		prev, curr = curr, prev
	}
	dist := prev[len(rb)]

	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	return 1 - float64(dist)/float64(maxLen)
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
