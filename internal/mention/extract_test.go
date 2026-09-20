package mention

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractAccountIDs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"none", "ただの本文です", nil},
		{"start of text", "@taro おはよう", []string{"taro"}},
		{"space before mention ok", "おはよう @taro", []string{"taro"}},
		{"needs space before marker", "あいう@taro", nil},
		{"email is not a mention", "連絡は mail@example.com まで", nil},
		{"terminates on japanese", "@taroさん ありがとう", []string{"taro"}},
		{"terminates on punctuation", "@taro、ありがとう", []string{"taro"}},
		{"newline terminates", "@taro\nありがとう", []string{"taro"}},
		{"underscore and hyphen are valid", "@ta_ro-2 です", []string{"ta_ro-2"}},
		{"fullwidth marker", "＠taro おはよう", []string{"taro"}},
		{"space after marker invalid", "@ taro", nil},
		{"multiple mentions", "@taro と @hanako へ", []string{"taro", "hanako"}},
		{"dedupe", "@taro と @taro", []string{"taro"}},
		{"dedupe ignores case", "@taro と @TARO", []string{"taro"}},
		{"truncated at 25 chars", "@" + strings.Repeat("a", 30), []string{strings.Repeat("a", 25)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractAccountIDs(tt.content, 0)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractAccountIDs(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// limit を超えたメンションは捨てる（通知爆撃の抑止）。
func TestExtractAccountIDsCapsAtLimit(t *testing.T) {
	const limit = 10
	var sb strings.Builder
	for i := 0; i < limit+5; i++ {
		sb.WriteString("@user")
		sb.WriteByte(byte('a' + i))
		sb.WriteString(" ")
	}
	got := ExtractAccountIDs(sb.String(), limit)
	if len(got) != limit {
		t.Errorf("len(ExtractAccountIDs(...)) = %d, want %d", len(got), limit)
	}
}
