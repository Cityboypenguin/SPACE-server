package post

import "regexp"

// hashtagRegex はハッシュタグを抽出する正規表現。
// 仕様:
//   - マーカーは "#"（半角ハッシュ）。直後に空白は挟まない（"#aaa" がハッシュタグ）。
//   - マーカーは本文先頭、または直前が空白（半角/全角スペース・タブ・改行）であること。
//     ("あいう#うえお" は反応せず、"あいう #うえお" のみ反応する)
//   - タグ本体はマーカー直後から、最初の空白（半角/全角スペース・タブ・改行）または
//     本文末尾までの「空白以外の連続」。
//
// \s は ASCII の空白（タブ/改行/スペース等）、\p{Zs} は Unicode の空白区切り
// （全角スペース U+3000 を含む）をカバーする。フロントエンドの正規表現と挙動を揃えている。
var hashtagRegex = regexp.MustCompile(`(?:^|[\s\p{Zs}])#([^\s\p{Zs}]+)`)

// ExtractHashtags は投稿本文からハッシュタグを抽出する。
// 出現順を保ちつつ重複を除去して返す。
func ExtractHashtags(content string) []string {
	matches := hashtagRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	tags := make([]string, 0, len(matches))
	for _, m := range matches {
		tag := m[1]
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}
