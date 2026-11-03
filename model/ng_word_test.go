package model

import (
	"testing"
	"time"
)

func matcherOf(t *testing.T, defs ...[2]string) *NGWordMatcher {
	t.Helper()
	var words []*NGWord
	for i, d := range defs {
		w, err := NewNGWord(d[1], NGWordMatchType(d[0]), time.Unix(1700000000, 0))
		if err != nil {
			t.Fatalf("NewNGWord(%q): %v", d[1], err)
		}
		w.ID = int64(i + 1)
		words = append(words, w)
	}
	return NewNGWordMatcher(words)
}

func TestNGWordMatcher(t *testing.T) {
	m := matcherOf(t,
		[2]string{"word", "Respon"},
		[2]string{"word", "死ね"},
		[2]string{"regex", `(^|\D)\d{9}($|\D)`},
	)
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"含む", "レスポンの Respon 共有します", true},
		{"大文字小文字を区別しない", "RESPON だよ", true},
		{"全角でもそろえて照合", "ＲＥＳＰＯＮ", true},
		{"見えない文字で割っても当たる", "Res" + string(rune(0x200b)) + "pon", true},
		{"日本語", "お前死ねよ", true},
		{"9桁の数字", "番号は 123456789 です", true},
		{"全角の9桁の数字", "番号は１２３４５６７８９です", true},
		{"8桁は当たらない", "12345678", false},
		{"10桁は当たらない（ちょうど9桁だけ）", "1234567890", false},
		{"当たらない", "今日の課題の範囲を教えてください", false},
		{"空", "", false},
	} {
		if got := m.Match(tc.text) != nil; got != tc.want {
			t.Errorf("%s: Match(%q) = %v, want %v", tc.name, tc.text, got, tc.want)
		}
	}
}

func TestNewNGWord_Validation(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name    string
		pattern string
		typ     NGWordMatchType
		ok      bool
	}{
		{"単語", "Respon", NGWordMatchWord, true},
		{"前後の空白は取る", "  Respon  ", NGWordMatchWord, true},
		{"空", "   ", NGWordMatchWord, false},
		{"長すぎる", string(make([]rune, 101)), NGWordMatchWord, false},
		{"正規表現", `\d{9}`, NGWordMatchRegex, true},
		{"壊れた正規表現", `(`, NGWordMatchRegex, false},
		{"空に当たる正規表現", `a*`, NGWordMatchRegex, false},
		{"未知の種類", "x", "glob", false},
	} {
		w, err := NewNGWord(tc.pattern, tc.typ, now)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
		if err == nil && tc.name == "前後の空白は取る" && w.Pattern != "Respon" {
			t.Errorf("pattern = %q, want trimmed", w.Pattern)
		}
	}
}

func TestNGWordMatcher_RegexIsWidthInsensitiveToo(t *testing.T) {
	m := matcherOf(t, [2]string{"regex", "Ｒｅｓｐｏｎ[0-9]+"})
	if m.Match("respon12") == nil || m.Match("ＲＥＳＰＯＮ１２") == nil {
		t.Error("a regex written in full-width characters must match both widths")
	}
}

func TestNGWordMatcher_SkipsBrokenRegex(t *testing.T) {
	m := NewNGWordMatcher([]*NGWord{
		{ID: 1, Pattern: "(", MatchType: NGWordMatchRegex},
		{ID: 2, Pattern: "abc", MatchType: NGWordMatchWord},
	})
	if m.Match("xxabcxx") == nil {
		t.Error("a broken regex must not disable the other words")
	}
}

func TestNGWordMatcherMask(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		rules            [][2]string
	}{
		{"multiple and repeated", "前RESPONと死ね、respon後", "前****と****、****後", [][2]string{{"word", "respon"}, {"word", "死ね"}}},
		{"width and invisible", "前ＲＥＳ\u200bＰＯＮ後", "前****後", [][2]string{{"word", "respon"}}},
		{"combined kana", "前ｶﾞとカ\u3099後", "前****と****後", [][2]string{{"word", "ガ"}}},
		{"expansion", "前㍿後", "前****後", [][2]string{{"word", "株式"}}},
		{"overlap", "前ababa後", "前****後", [][2]string{{"word", "aba"}}},
		{"overlapping rules", "前abcd後", "前****後", [][2]string{{"word", "abc"}, {"word", "bcd"}}},
		{"regex", "番号１２３４５６７８９です", "番号****です", [][2]string{{"regex", `[0-9]{9}`}}},
		{"clean preserved", "  ＡＢＣ と😀  ", "  ＡＢＣ と😀  ", [][2]string{{"word", "respon"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matcherOf(t, tc.rules...).Mask(tc.text); got != tc.want {
				t.Fatalf("Mask(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
