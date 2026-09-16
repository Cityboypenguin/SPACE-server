// Package mention はメンションの「記法」だけを担当する。
//
// 誰に通知するか・誰をメンションできるかというポリシーは usecase 側に置き、
// ここには「本文のどこからどこまでがメンションか」の判定だけを置く。
// 投稿 (@accountID) とコミュニティ (@表示名) で判定の入口は違うが、
// マーカーと位置の条件は共通なので、二重定義にならないようここへ集約している。
// フロントエンドの src/lib/mentions.ts と挙動を揃えること。
package mention

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// マーカーは半角 "@" と全角 "＠" の両方を受ける（日本語入力だと全角になりやすいため）。
var markers = []string{"@", "＠"}

// accountIDMentionRegex は投稿本文から "@accountID" を抜き出す。
// 仕様:
//   - マーカーは本文先頭、または直前が空白（半角/全角スペース・タブ・改行）であること。
//     ("mail@example.com" は直前が空白でないため反応しない)
//   - 本体は accountID として有効な文字（model.ValidateAccountID と同じ [a-zA-Z0-9_-]）の
//     連続で、最大 25 文字（model.MaxAccountIDLength）。
//     ハッシュタグのように「空白まで」ではなく文字種で終端するため、
//     "@taro さん" だけでなく "@taroさん" や "@taro、" も正しく切れる。
//
// \s は ASCII の空白、\p{Zs} は Unicode の空白区切り（全角スペース U+3000 を含む）。
var accountIDMentionRegex = regexp.MustCompile(`(?:^|[\s\p{Zs}])[@＠]([a-zA-Z0-9_-]{1,25})`)

// ExtractAccountIDs は投稿本文からメンション先の accountID を抽出する。
// 出現順を保ちつつ重複を除去し、limit 件までを返す（limit <= 0 なら無制限）。
// 大文字小文字はここでは畳まない（解決時に DB の照合順序に委ねる）が、
// 同じ相手を大文字小文字違いで二重に数えないよう重複判定だけは小文字で行う。
func ExtractAccountIDs(content string, limit int) []string {
	matches := accountIDMentionRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	accountIDs := make([]string, 0, len(matches))
	for _, m := range matches {
		accountID := m[1]
		key := strings.ToLower(accountID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		accountIDs = append(accountIDs, accountID)
		if limit > 0 && len(accountIDs) >= limit {
			break
		}
	}
	return accountIDs
}

// ContainsText は本文に "@<text>" がメンションとして書かれているかを判定する。
// 位置の条件は ExtractAccountIDs と同じ（本文先頭、または直前が空白）。
// text は表示名でありメタ文字を含みうるため、正規表現ではなく素朴な走査で判定する。
func ContainsText(content, text string) bool {
	if text == "" {
		return false
	}

	for _, marker := range markers {
		token := marker + text
		offset := 0
		for {
			idx := strings.Index(content[offset:], token)
			if idx < 0 {
				break
			}
			pos := offset + idx
			if isBoundary(content, pos) {
				return true
			}
			// 同じマーカーで始まる次の候補から探し直す。
			offset = pos + len(marker)
		}
	}
	return false
}

// isBoundary は index のマーカーがメンションとして有効な位置か
// （本文先頭、または直前が空白）を返す。
func isBoundary(content string, index int) bool {
	if index == 0 {
		return true
	}
	r, size := utf8.DecodeLastRuneInString(content[:index])
	if size == 0 {
		return false
	}
	return unicode.IsSpace(r)
}
