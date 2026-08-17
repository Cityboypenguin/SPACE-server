package post

import (
	"reflect"
	"testing"
)

func TestExtractHashtags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"none", "ただの本文です", nil},
		{"start of text", "#東京 に行った", []string{"東京"}},
		{"newline terminates", "#東京\n楽しかった", []string{"東京"}},
		{"space terminates", "#東京 旅行", []string{"東京"}},
		{"needs space before hash", "あいう#うえお かきく", nil},
		{"space before hash ok", "あいう #うえお かきく", []string{"うえお"}},
		{"space after hash invalid", "# 東京", nil},
		{"hash with no body", "あいう # うえお", nil},
		{"multiple tags", "#東京\n#旅行 最高", []string{"東京", "旅行"}},
		{"dedupe", "#東京 です\n#東京", []string{"東京"}},
		{"fullwidth space terminates", "#東京　旅行", []string{"東京"}},
		{"two tags same line", "行った #渋谷 と #新宿", []string{"渋谷", "新宿"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractHashtags(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractHashtags(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}
