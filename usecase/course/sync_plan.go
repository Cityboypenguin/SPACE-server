package course

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/model"
)

// ■ シラバス同期の照合（DB に触らない部分）
//
// 取り込み元（シラバス）の今の内容と DB の授業を突き合わせて、何を作り・変え・廃止し・
// 管理者の確認に回すかを決める。書き込みは SyncCoursesInteractor が計画どおりに行う。
// ここを純粋な関数にしてあるのは、照合の規則をテーブルテストで確かめるため。
//
// 授業は (学期, 講義コード) ごとにまとめて比べる。1つの講義コードが週に複数コマ
// 持つことがある（parseRows のコメント参照）ので、まとまり単位で対応を付ける。
//
//   - 曜日・時限まで一致する行同士はそのまま対応させ、授業名・教員名の差を更新する
//   - 残った行が旧1・新1なら同じ授業のコマ変更として更新する（水5 → 水4）
//   - 旧が残らなければ新規作成、新が残らなければ廃止候補
//   - それ以外（旧2・新3など）は対応が決まらないので確認に回す
//   - 授業名と教員名が両方変わっていたら講義コードの使い回しを疑い、まとまりごと確認に回す
//   - 消えた講義コードと同じ授業名・教員名で新しい講義コードが1対1で現れたら、
//     振り直しを疑って確認に回す
//
// 廃止候補は最後にまとめて判定し、取りこぼしの疑いがあるときや多すぎるときは
// 1件も廃止しない（discontinueGuard）。

// discontinueMaxRatioPercent は1回の同期で廃止してよい授業の割合の上限（現行の授業数に対する%）。
// これを超えるときはサイト側の不具合や仕様変更を疑い、廃止を見送る。
const discontinueMaxRatioPercent = 10

// FetchCompleteness はシラバスを取りこぼしなく読めたかの手がかり。
type FetchCompleteness struct {
	// SiteTotal はサイトが報告した授業の件数（一覧の「N件中」）。
	SiteTotal int
	// ListedRows は実際に一覧から読めた授業の行数。
	ListedRows int
	// UnidentifiedRows は講義コードを読み取れなかった行数。その授業は照合できないので、
	// 1件でもあれば廃止を見送る（在るのに「消えた」と判定しかねない）。
	UnidentifiedRows int
}

// CourseFields は同期で書き換える授業の中身。
type CourseFields struct {
	Semester    string
	DayOfWeek   string
	Period      int
	CourseName  string
	TeacherName string
	SourceRef   string
	SourceName  string
	DedupKey    string
}

// PlannedUpdate は既存の授業1件の更新。
type PlannedUpdate struct {
	Before *model.Course
	After  CourseFields
	// Restore は廃止済みの授業がシラバスに戻ってきたので廃止の印を外す。
	Restore bool
	// Silent は表に出ない列（source_name の補完、dedup_key）だけの更新。
	// 変更の記録には載せない。
	Silent      bool
	SlotChanged bool
	Detail      string
}

// PlannedCreate は新しく作る授業1件。
type PlannedCreate struct {
	Input  ScrapedCourseInput
	Detail string
}

// PlannedDiscontinue は廃止する授業1件。
type PlannedDiscontinue struct {
	Course *model.Course
	// Retire は管理者が「講義コードの使い回し＝別の授業」と判断したもの。廃止に加えて
	// 照合から外し（source_ref を空にする）、同じ dedup_key で新しい授業を作れるようにする。
	Retire bool
	Detail string
}

// PlannedReview は管理者の確認に回す変化1件。
type PlannedReview struct {
	Kind        model.CourseSyncReviewKind
	Fingerprint string
	Message     string
	Existing    []*model.Course
	Proposed    []ScrapedCourseInput
}

// SyncPlan は同期1回で行うことの一覧。
type SyncPlan struct {
	Creates      []PlannedCreate
	Updates      []PlannedUpdate
	Discontinues []PlannedDiscontinue
	Reviews      []PlannedReview
	// AppliedReviewIDs はこの同期で管理者の判断（SAME / DIFFERENT）を反映した確認。
	AppliedReviewIDs []int64
	// Unchanged はシラバスにあって何も変わらなかった授業の数。
	Unchanged int
	// DiscontinueSkippedReason は廃止を見送った理由。空なら見送っていない。
	DiscontinueSkippedReason string
}

// syncGroupKey は照合の単位（学期と講義コード）。年度は同期の単位なので含めない。
type syncGroupKey struct {
	semester  string
	sourceRef string
}

func (k syncGroupKey) String() string { return k.semester + ":" + k.sourceRef }

func slotLabel(day string, period int) string { return day + strconv.Itoa(period) }

// PlanCourseSync は year の授業について、シラバスの内容（scraped）と DB の授業
// （existing: その年度の取り込み元が同じ授業。廃止済みも含む）を突き合わせる。
// reviews はその年度の確認（fingerprint で引く）で、管理者の判断を反映するのに使う。
func PlanCourseSync(year int, existing []*model.Course, scraped []ScrapedCourseInput, reviews []*model.CourseSyncReview, completeness FetchCompleteness) *SyncPlan {
	p := &planner{
		year:      year,
		plan:      &SyncPlan{},
		decisions: make(map[string]*model.CourseSyncReview, len(reviews)),
	}
	for _, r := range reviews {
		p.decisions[r.Fingerprint] = r
	}

	oldGroups := make(map[syncGroupKey][]*model.Course)
	activeCount := 0
	for _, c := range existing {
		if c.SourceRef == "" {
			continue
		}
		k := syncGroupKey{c.Semester, c.SourceRef}
		oldGroups[k] = append(oldGroups[k], c)
		if !c.IsDiscontinued() {
			activeCount++
		}
	}

	newGroups := make(map[syncGroupKey][]ScrapedCourseInput)
	seenKeys := make(map[string]bool, len(scraped))
	for _, in := range scraped {
		// 同じ取り込みの中に同じ行が2回出てくることがある（スクレイピング側の重複行）。
		if seenKeys[in.DedupKey] {
			continue
		}
		seenKeys[in.DedupKey] = true
		k := syncGroupKey{in.Semester, in.SourceRef}
		newGroups[k] = append(newGroups[k], in)
	}

	// 1. 講義コードが DB にもシラバスにもあるまとまり。
	var newOnly []syncGroupKey
	for _, k := range sortedKeys(newGroups) {
		if olds, ok := oldGroups[k]; ok {
			p.planGroup(k, olds, newGroups[k], false)
			delete(oldGroups, k)
			continue
		}
		newOnly = append(newOnly, k)
	}

	// 2. シラバスにだけある講義コード。消えた講義コードと同じ授業名・教員名なら
	//    振り直しを疑う。1対1に決まるときだけ（同じ授業名・教員名の授業が複数あると
	//    どれがどれか分からないので、新規と廃止として扱う）。
	reissue := p.matchReissues(newOnly, newGroups, oldGroups)
	for _, k := range newOnly {
		news := newGroups[k]
		oldKey, ok := reissue[k]
		if !ok {
			p.createAll(news, "")
			continue
		}
		olds := oldGroups[oldKey]
		fp := fingerprint(model.CourseSyncReviewCodeReissued, year, oldKey.String(), k.String())
		switch decision := p.decisions[fp]; decisionStatus(decision) {
		case model.CourseSyncReviewSame:
			delete(oldGroups, oldKey)
			p.planGroup(k, olds, news, true)
			p.applied(decision)
		case model.CourseSyncReviewDifferent:
			p.createAll(news, "講義コードの振り直しではないと判断（管理者）")
			p.applied(decision)
		case model.CourseSyncReviewIgnored:
			// 据え置き。消えた側も廃止しない。
			delete(oldGroups, oldKey)
		default:
			delete(oldGroups, oldKey)
			p.review(model.CourseSyncReviewCodeReissued, fp,
				fmt.Sprintf("講義コード %s が消え、同じ授業名・教員名で講義コード %s が現れました。講義コードが振り直された可能性があります。",
					oldKey.sourceRef, k.sourceRef),
				olds, news)
		}
	}

	// 3. シラバスから消えた講義コード。
	for _, k := range sortedKeys(oldGroups) {
		for _, c := range oldGroups[k] {
			p.discontinueCandidate(c)
		}
	}

	p.decideDiscontinues(activeCount, completeness)
	return p.plan
}

type planner struct {
	year       int
	plan       *SyncPlan
	decisions  map[string]*model.CourseSyncReview
	candidates []*model.Course
	// forced は管理者の判断による廃止。見送りの対象にしない。
	forced []PlannedDiscontinue
}

func decisionStatus(r *model.CourseSyncReview) model.CourseSyncReviewStatus {
	if r == nil {
		return ""
	}
	return r.Status
}

func (p *planner) applied(r *model.CourseSyncReview) {
	p.plan.AppliedReviewIDs = append(p.plan.AppliedReviewIDs, r.ID)
}

// planGroup は同じ (学期, 講義コード) の旧（DB）と新（シラバス）を対応させる。
// skipReuseCheck は管理者が既に「同じ授業」と判断したまとまりで、使い回しの疑いを
// 改めて確かめない。
func (p *planner) planGroup(k syncGroupKey, olds []*model.Course, news []ScrapedCourseInput, skipReuseCheck bool) {
	if !skipReuseCheck && !nameMatches(olds[0], news[0].SourceName) && olds[0].TeacherName != news[0].TeacherName {
		fp := fingerprint(model.CourseSyncReviewCodeReused, p.year, k.String(), news[0].SourceName, news[0].TeacherName)
		switch decision := p.decisions[fp]; decisionStatus(decision) {
		case model.CourseSyncReviewSame:
			p.applied(decision)
		case model.CourseSyncReviewDifferent:
			for _, c := range olds {
				p.forced = append(p.forced, PlannedDiscontinue{Course: c, Retire: true, Detail: "講義コードが別の授業に使い回されたと判断（管理者）"})
			}
			p.createAll(news, "講義コードが別の授業に使い回されたと判断（管理者）")
			p.applied(decision)
			return
		case model.CourseSyncReviewIgnored:
			return
		default:
			p.review(model.CourseSyncReviewCodeReused, fp,
				fmt.Sprintf("講義コード %s の授業名と教員名が両方変わりました（%s・%s → %s・%s）。講義コードが別の授業に使い回された可能性があります。",
					k.sourceRef, olds[0].CourseName, olds[0].TeacherName, news[0].SourceName, news[0].TeacherName),
				olds, news)
			return
		}
	}

	oldBySlot := make(map[string]*model.Course, len(olds))
	for _, c := range olds {
		oldBySlot[slotLabel(c.DayOfWeek, c.Period)] = c
	}
	matched := make(map[int64]bool, len(olds))
	var leftoverNew []ScrapedCourseInput
	for _, in := range news {
		if c, ok := oldBySlot[slotLabel(in.DayOfWeek, in.Period)]; ok && !matched[c.ID] {
			matched[c.ID] = true
			p.update(c, in)
			continue
		}
		leftoverNew = append(leftoverNew, in)
	}
	var leftoverOld []*model.Course
	for _, c := range olds {
		if !matched[c.ID] {
			leftoverOld = append(leftoverOld, c)
		}
	}

	switch {
	case len(leftoverOld) == 0:
		p.createAll(leftoverNew, "")
	case len(leftoverNew) == 0:
		for _, c := range leftoverOld {
			p.discontinueCandidate(c)
		}
	case len(leftoverOld) == 1 && len(leftoverNew) == 1:
		p.update(leftoverOld[0], leftoverNew[0])
	default:
		fp := fingerprint(model.CourseSyncReviewSlotAmbiguous, p.year, k.String(),
			strings.Join(oldSlots(leftoverOld), ","), strings.Join(newSlots(leftoverNew), ","))
		switch decision := p.decisions[fp]; decisionStatus(decision) {
		case model.CourseSyncReviewDifferent:
			for _, c := range leftoverOld {
				if !c.IsDiscontinued() {
					p.forced = append(p.forced, PlannedDiscontinue{Course: c, Detail: "コマの変更を作り直しと判断（管理者）"})
				}
			}
			p.createAll(leftoverNew, "コマの変更を作り直しと判断（管理者）")
			p.applied(decision)
		case model.CourseSyncReviewIgnored:
		default:
			p.review(model.CourseSyncReviewSlotAmbiguous, fp,
				fmt.Sprintf("講義コード %s のコマが %s から %s に変わりました。どのコマがどのコマに移ったのか決められません。",
					k.sourceRef, strings.Join(oldSlots(leftoverOld), "・"), strings.Join(newSlots(leftoverNew), "・")),
				leftoverOld, leftoverNew)
		}
	}
}

// update は旧 c を新 in の内容に合わせる。差が無ければ Unchanged に数える。
func (p *planner) update(c *model.Course, in ScrapedCourseInput) {
	after := CourseFields{
		Semester:    in.Semester,
		DayOfWeek:   in.DayOfWeek,
		Period:      in.Period,
		CourseName:  nextCourseName(c, in),
		TeacherName: in.TeacherName,
		SourceRef:   in.SourceRef,
		SourceName:  in.SourceName,
		DedupKey:    in.DedupKey,
	}

	var details []string
	if c.SourceRef != after.SourceRef {
		details = append(details, fmt.Sprintf("講義コード: %s → %s", c.SourceRef, after.SourceRef))
	}
	if c.Semester != after.Semester {
		details = append(details, fmt.Sprintf("学期: %s → %s", c.Semester, after.Semester))
	}
	slotChanged := c.DayOfWeek != after.DayOfWeek || c.Period != after.Period
	if slotChanged {
		details = append(details, fmt.Sprintf("コマ: %s → %s", slotLabel(c.DayOfWeek, c.Period), slotLabel(after.DayOfWeek, after.Period)))
	}
	if c.CourseName != after.CourseName {
		details = append(details, fmt.Sprintf("授業名: %s → %s", c.CourseName, after.CourseName))
	}
	if c.TeacherName != after.TeacherName {
		details = append(details, fmt.Sprintf("教員: %s → %s", c.TeacherName, after.TeacherName))
	}
	restore := c.IsDiscontinued()
	if restore {
		details = append([]string{"シラバスに再び掲載されたため廃止を取り消し"}, details...)
	}

	silent := len(details) == 0
	if silent && c.SourceName == after.SourceName && c.DedupKey == after.DedupKey {
		p.plan.Unchanged++
		return
	}
	if silent {
		p.plan.Unchanged++
	}
	p.plan.Updates = append(p.plan.Updates, PlannedUpdate{
		Before:      c,
		After:       after,
		Restore:     restore,
		Silent:      silent,
		SlotChanged: slotChanged,
		Detail:      strings.Join(details, " / "),
	})
}

func (p *planner) createAll(news []ScrapedCourseInput, detail string) {
	for _, in := range news {
		p.plan.Creates = append(p.plan.Creates, PlannedCreate{Input: in, Detail: detail})
	}
}

func (p *planner) discontinueCandidate(c *model.Course) {
	if c.IsDiscontinued() {
		return
	}
	p.candidates = append(p.candidates, c)
}

func (p *planner) review(kind model.CourseSyncReviewKind, fp, message string, olds []*model.Course, news []ScrapedCourseInput) {
	p.plan.Reviews = append(p.plan.Reviews, PlannedReview{
		Kind:        kind,
		Fingerprint: fp,
		Message:     message,
		Existing:    olds,
		Proposed:    news,
	})
}

// matchReissues は新しく現れた講義コードと消えた講義コードのうち、授業名・教員名で
// 1対1に対応するものを返す（新 → 旧）。消えた側は現行の授業を含むものだけを見る
// （とうに廃止した授業まで候補にすると、確認ばかり増える）。
func (p *planner) matchReissues(newOnly []syncGroupKey, newGroups map[syncGroupKey][]ScrapedCourseInput, gone map[syncGroupKey][]*model.Course) map[syncGroupKey]syncGroupKey {
	candidatesOf := make(map[syncGroupKey][]syncGroupKey)
	claimedBy := make(map[syncGroupKey]int)
	for _, nk := range newOnly {
		in := newGroups[nk][0]
		for _, ok := range sortedKeys(gone) {
			olds := gone[ok]
			if !hasActive(olds) || olds[0].TeacherName != in.TeacherName || !nameMatches(olds[0], in.SourceName) {
				continue
			}
			candidatesOf[nk] = append(candidatesOf[nk], ok)
			claimedBy[ok]++
		}
	}
	out := make(map[syncGroupKey]syncGroupKey)
	for nk, cands := range candidatesOf {
		if len(cands) == 1 && claimedBy[cands[0]] == 1 {
			out[nk] = cands[0]
		}
	}
	return out
}

// decideDiscontinues は廃止候補を本当に廃止するか決める。
func (p *planner) decideDiscontinues(activeCount int, completeness FetchCompleteness) {
	p.plan.Discontinues = append(p.plan.Discontinues, p.forced...)
	if len(p.candidates) == 0 {
		return
	}
	reason := discontinueGuard(len(p.candidates), activeCount, completeness)
	if reason != "" {
		p.plan.DiscontinueSkippedReason = reason
		return
	}
	for _, c := range p.candidates {
		p.plan.Discontinues = append(p.plan.Discontinues, PlannedDiscontinue{Course: c, Detail: "シラバスに掲載されなくなったため廃止"})
	}
}

// discontinueGuard は廃止を見送るべき理由を返す（見送らないなら空）。
func discontinueGuard(candidates, activeCount int, completeness FetchCompleteness) string {
	if completeness.ListedRows != completeness.SiteTotal {
		return fmt.Sprintf("一覧から読めた授業（%d件）がサイトの件数（%d件）と一致しないため、取りこぼしを疑って廃止を見送りました（廃止候補 %d件）",
			completeness.ListedRows, completeness.SiteTotal, candidates)
	}
	if completeness.UnidentifiedRows > 0 {
		return fmt.Sprintf("講義コードを読み取れない授業が%d件あったため、廃止を見送りました（廃止候補 %d件）",
			completeness.UnidentifiedRows, candidates)
	}
	if candidates*100 > activeCount*discontinueMaxRatioPercent {
		return fmt.Sprintf("廃止候補が%d件で、現行の授業（%d件）の%d%%を超えるため、サイト側の不具合や仕様変更を疑って廃止を見送りました",
			candidates, activeCount, discontinueMaxRatioPercent)
	}
	return ""
}

// nameMatches は DB の授業 c の授業名が、シラバスの授業名 sourceName と同じとみなせるか。
// 同期前から在る行は source_name が無いので、校舎サフィックス「（…）」付きの名前でも
// 先頭が一致すれば同じとみなす。
func nameMatches(c *model.Course, sourceName string) bool {
	if c.SourceName != "" {
		return c.SourceName == sourceName
	}
	return c.CourseName == sourceName || strings.HasPrefix(c.CourseName, sourceName+"（")
}

// nextCourseName は更新後の表示名を決める。
//
// スクレイパーは、同じ授業名・教員名・コマの行が複数あるときだけ詳細ページを見て
// 校舎サフィックスを付ける。しかも既知の授業ばかりのまとまりではその確認を省く
// （FetchCourses の knownDedupKeys）。省かれた行の CourseName はサフィックス無しなので、
// そのまま書くと付いていたサフィックスが消えてしまう。そこで:
//   - 今回スクレイパーが確かめた行（Disambiguated）はその名前が正
//   - 確かめていない行は、授業名が変わっていなければ今の表示名を保ち、変わっていれば
//     新しい授業名に今のサフィックスを付け直す
func nextCourseName(c *model.Course, in ScrapedCourseInput) string {
	if in.Disambiguated {
		return in.CourseName
	}
	if nameMatches(c, in.SourceName) {
		return c.CourseName
	}
	suffix := ""
	if c.SourceName != "" && strings.HasPrefix(c.CourseName, c.SourceName) {
		suffix = strings.TrimPrefix(c.CourseName, c.SourceName)
	}
	return in.SourceName + suffix
}

func hasActive(cs []*model.Course) bool {
	for _, c := range cs {
		if !c.IsDiscontinued() {
			return true
		}
	}
	return false
}

func oldSlots(cs []*model.Course) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, slotLabel(c.DayOfWeek, c.Period))
	}
	sort.Strings(out)
	return out
}

func newSlots(ins []ScrapedCourseInput) []string {
	out := make([]string, 0, len(ins))
	for _, in := range ins {
		out = append(out, slotLabel(in.DayOfWeek, in.Period))
	}
	sort.Strings(out)
	return out
}

// fingerprint は「同じ状況」を表す値。再実行で同じ確認を積み増さないため、
// 管理者の判断を次の同期で引き当てるために使う。
func fingerprint(kind model.CourseSyncReviewKind, year int, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(string(kind)))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(year)))
	for _, part := range parts {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sortedKeys[V any](m map[syncGroupKey]V) []syncGroupKey {
	keys := make([]syncGroupKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].semester != keys[j].semester {
			return keys[i].semester < keys[j].semester
		}
		return keys[i].sourceRef < keys[j].sourceRef
	})
	return keys
}
