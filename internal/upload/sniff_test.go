package upload

import (
	"strings"
	"testing"
)

// 申告された種別と中身の一致を、形式ごとに確かめる。
//
// 署名付き PUT では Content-Type も利用者が決めるので、保存されたメタデータは
// 「そう名乗った」という記録にすぎない。ここが機能していないと、中身が HTML の
// ファイルを image/png と名乗って置ける。
func TestContentTypeMatchesBytes(t *testing.T) {
	zeros := strings.Repeat("\x00", 24)

	tests := []struct {
		name     string
		declared string
		body     string
		want     bool
	}{
		{"PNG を PNG と名乗る", "image/png", "\x89PNG\r\n\x1a\n" + zeros, true},
		{"JPEG を JPEG と名乗る", "image/jpeg", "\xff\xd8\xff" + zeros, true},
		{"GIF を GIF と名乗る", "image/gif", "GIF89a" + zeros, true},
		{"WebP を WebP と名乗る", "image/webp", "RIFF\x00\x00\x00\x00WEBPVP8 " + zeros, true},

		// ここが本題。中身と名乗りが食い違うものを通さない。
		{"HTML を PNG と名乗る", "image/png", "<!DOCTYPE html><html><body>x</body></html>", false},
		{"HTML を SVG と名乗る", "image/svg+xml", "<!DOCTYPE html><html><body>x</body></html>", false},
		{"HTML をマークダウンと名乗る", "text/markdown", "<div>x</div>", false},
		{"JPEG を PNG と名乗る", "image/png", "\xff\xd8\xff" + zeros, false},
		{"空の中身", "image/png", "", false},

		// SVG とマークダウンはテキストなのでバイト列で一致を見られない。
		// SVG は根の要素、マークダウンは「HTML として解釈されないこと」で見る。
		{"素の SVG", "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`, true},
		{"宣言付きの SVG", "image/svg+xml", `<?xml version="1.0"?><svg><rect/></svg>`, true},
		{"コメント付きの SVG", "image/svg+xml", "<!-- 作成: 何か -->\n<svg><rect/></svg>", true},
		{"根が svg でない XML", "image/svg+xml", `<?xml version="1.0"?><note><to>x</to></note>`, false},
		{"XML として壊れている", "image/svg+xml", "<svg", false},
		{"ふつうのマークダウン", "text/markdown", "# 利用規約\n\n第1条 ...\n", true},

		// script が入っていても「SVG ではある」ので true。
		// SVG が持つ危険はこの層では落とせない（配信側と別ホストで受ける）。
		{"script 入りの SVG も形式としては SVG", "image/svg+xml", `<svg><script>alert(1)</script></svg>`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentTypeMatchesBytes(tt.declared, []byte(tt.body)); got != tt.want {
				t.Errorf("contentTypeMatchesBytes(%q, %q) = %v, want %v",
					tt.declared, truncate(tt.body), got, tt.want)
			}
		})
	}
}

// 根の要素が先頭 sniffLimit バイトより後ろにあるものは受け付けない。
//
// 判定できないものを通すと、判定そのものが素通しになる。
func TestSVGWithRootElementBeyondTheSniffLimitIsRejected(t *testing.T) {
	body := "<!--" + strings.Repeat("あ", sniffLimit) + "-->\n<svg></svg>"
	if contentTypeMatchesBytes("image/svg+xml", []byte(body)[:sniffLimit]) {
		t.Error("根の要素まで届いていないのに受け入れてしまった")
	}
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}
