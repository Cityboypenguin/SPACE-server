package mention

import "testing"

func TestContainsText(t *testing.T) {
	tests := []struct {
		name    string
		content string
		text    string
		want    bool
	}{
		{"start of text", "@山田 太郎 確認おねがいします", "山田 太郎", true},
		{"after space", "これ @山田 太郎 見てください", "山田 太郎", true},
		{"needs space before marker", "あいう@山田", "山田", false},
		{"fullwidth marker", "＠山田 です", "山田", true},
		{"fullwidth space before marker", "どうぞ　@山田", "山田", true},
		{"newline before marker", "本文\n@山田", "山田", true},
		{"not present", "だれもメンションしていません", "山田", false},
		{"empty name", "@ です", "", false},
		{"partial name is still a prefix match", "@山田太郎 です", "山田", true},
		{"second occurrence is valid", "mail@yamada と @山田", "山田", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsText(tt.content, tt.text); got != tt.want {
				t.Errorf("ContainsText(%q, %q) = %v, want %v", tt.content, tt.text, got, tt.want)
			}
		})
	}
}
