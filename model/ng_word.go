package model

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

// NGWordMatchType はNGワードの照合のしかた。
type NGWordMatchType string

const (
	// NGWordMatchWord は「含む」。大小・全半角は区別しない。
	NGWordMatchWord NGWordMatchType = "word"
	// NGWordMatchRegex は正規表現（RE2）。例: 9桁の数字 (^|\D)\d{9}($|\D)
	NGWordMatchRegex NGWordMatchType = "regex"
)

// NGWordSource はどの投稿を止めたか。
type NGWordSource string

const (
	NGWordSourceMessage  NGWordSource = "message"
	NGWordSourceQuestion NGWordSource = "question"
	NGWordSourceAnswer   NGWordSource = "answer"
	// NGWordSourceAddendum は質問・回答への追記。
	NGWordSourceAddendum NGWordSource = "addendum"
	NGWordSourceThread   NGWordSource = "thread"
	NGWordSourcePoll     NGWordSource = "poll"
)

const (
	MaxNGWordPatternChars = 100
	// MaxNGWords は登録できる数の上限。投稿のたびに全部を照合するので、無制限にはしない。
	MaxNGWords = 200
)

type NGWord struct {
	ID        int64
	Pattern   string
	MatchType NGWordMatchType
	CreatedAt time.Time
}

// NGWordHit は投稿を止めた1回の記録。
type NGWordHit struct {
	NGWordID  *int64
	Pattern   string
	UserID    int64
	RoomID    *int64
	Source    NGWordSource
	CreatedAt time.Time
}

// NGWordHitStats は止めた回数の集計。
type NGWordHitStats struct {
	Total  int
	ByWord []NGWordHitCount
	ByUser []NGWordUserHitCount
}

type NGWordHitCount struct {
	Pattern string
	Count   int
}

type NGWordUserHitCount struct {
	UserID    int64
	Name      string
	AccountID string
	Count     int
}

// zeroWidth は見えない文字。間に挟んで検査をすり抜ける手を防ぐため、照合の前に取り除く。
var zeroWidth = strings.NewReplacer(string(rune(0x200b)), "", string(rune(0x200c)), "", string(rune(0x200d)), "", string(rune(0x2060)), "", string(rune(0xfeff)), "")

// NormalizeForNGWord は照合用に文面をそろえる（全半角・合成文字をそろえ、小文字にし、見えない文字を除く）。
func NormalizeForNGWord(s string) string {
	return strings.ToLower(zeroWidth.Replace(norm.NFKC.String(s)))
}

// NewNGWord は入力を検証してNGワードを作る。
func NewNGWord(pattern string, matchType NGWordMatchType, now time.Time) (*NGWord, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, apperr.InvalidInput("NGワードを入力してください")
	}
	if utf8.RuneCountInString(pattern) > MaxNGWordPatternChars {
		return nil, apperr.InvalidInput("NGワードは100文字以内で入力してください")
	}
	switch matchType {
	case NGWordMatchWord:
	case NGWordMatchRegex:
		re, err := compileNGRegex(pattern)
		if err != nil {
			return nil, apperr.InvalidInput("正規表現が正しくありません")
		}
		// 空の文字列に当たるパターン（例: a*）は、すべての投稿を止めてしまう。
		if re.MatchString("") {
			return nil, apperr.InvalidInput("すべての文字列に当たる正規表現は登録できません")
		}
	default:
		return nil, apperr.InvalidInput("照合のしかたが正しくありません")
	}
	return &NGWord{Pattern: pattern, MatchType: matchType, CreatedAt: now}, nil
}

func compileNGRegex(pattern string) (*regexp.Regexp, error) {
	// パターンは全角・半角だけそろえる（小文字にすると \D が \d になって意味が変わるので、
	// 大小は (?i) で吸収する）。文面のほうは NormalizeForNGWord でそろえてから照合する。
	return regexp.Compile("(?i)" + norm.NFKC.String(pattern))
}

type compiledNGWord struct {
	word   *NGWord
	needle string
	re     *regexp.Regexp
}

// NGWordMatcher は登録済みのNGワードを照合用に前処理したもの。
type NGWordMatcher struct {
	words []compiledNGWord
}

// NewNGWordMatcher は照合器を作る。正規表現が壊れている行は飛ばす（1件の不良で投稿全体を止めない）。
func NewNGWordMatcher(words []*NGWord) *NGWordMatcher {
	m := &NGWordMatcher{}
	for _, w := range words {
		c := compiledNGWord{word: w}
		switch w.MatchType {
		case NGWordMatchRegex:
			re, err := compileNGRegex(w.Pattern)
			if err != nil {
				continue
			}
			c.re = re
		default:
			c.needle = NormalizeForNGWord(w.Pattern)
			if c.needle == "" {
				continue
			}
		}
		m.words = append(m.words, c)
	}
	return m
}

// Match は text に当たった最初のNGワードを返す（無ければ nil）。
func (m *NGWordMatcher) Match(text string) *NGWord {
	if m == nil || text == "" {
		return nil
	}
	normalized := NormalizeForNGWord(text)
	for _, c := range m.words {
		if c.re != nil {
			if c.re.MatchString(normalized) {
				return c.word
			}
			continue
		}
		if strings.Contains(normalized, c.needle) {
			return c.word
		}
	}
	return nil
}

// Mask replaces every matching span while keeping the original spelling of other text.
// NFKC segments map normalized byte offsets back to the original input, including
// combined kana, compatibility expansions, and invisible characters inside a match.
func (m *NGWordMatcher) Mask(text string) string {
	if m == nil || text == "" {
		return text
	}
	type span struct{ start, end int }
	var normalized strings.Builder
	var origins []span
	for start := 0; start < len(text); {
		size := norm.NFKC.NextBoundaryInString(text[start:], true)
		if size <= 0 {
			size = len(text) - start
		}
		end := start + size
		segment := NormalizeForNGWord(text[start:end])
		normalized.WriteString(segment)
		for range len(segment) {
			origins = append(origins, span{start, end})
		}
		start = end
	}
	value := normalized.String()
	var spans []span
	for _, c := range m.words {
		var matches [][]int
		if c.re != nil {
			matches = c.re.FindAllStringIndex(value, -1)
		} else {
			// Advance one rune so overlapping occurrences are also masked.
			for offset := 0; offset < len(value); {
				index := strings.Index(value[offset:], c.needle)
				if index < 0 {
					break
				}
				start := offset + index
				matches = append(matches, []int{start, start + len(c.needle)})
				_, size := utf8.DecodeRuneInString(value[start:])
				offset = start + size
			}
		}
		for _, match := range matches {
			if match[0] < match[1] {
				spans = append(spans, span{origins[match[0]].start, origins[match[1]-1].end})
			}
		}
	}
	if len(spans) == 0 {
		return text
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	merged := []span{spans[0]}
	for _, next := range spans[1:] {
		last := &merged[len(merged)-1]
		if next.start < last.end {
			if next.end > last.end {
				last.end = next.end
			}
		} else {
			merged = append(merged, next)
		}
	}
	var result strings.Builder
	offset := 0
	for _, match := range merged {
		result.WriteString(text[offset:match.start])
		result.WriteString("****")
		offset = match.end
	}
	result.WriteString(text[offset:])
	return result.String()
}
