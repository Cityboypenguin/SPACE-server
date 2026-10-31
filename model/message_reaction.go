package model

import "strings"

// MessageReaction は1つのメッセージに付いた、1種類の絵文字ぶんのリアクションの集計。
// 1人が複数の絵文字を付けられるが、同じ絵文字は1人1回まで。
type MessageReaction struct {
	Emoji string
	Count int
	// ReactedByMe は閲覧者がこの絵文字を付けているか。閲覧者ごとに変わる値なので、
	// 保存はせず取得のたびに閲覧者のユーザーIDで解決する。
	ReactedByMe bool
}

// StampIDPrefix は文字スタンプのIDの接頭辞。
const StampIDPrefix = "stamp:"

// IsAllowedMessageStamp は id が、メッセージとして単体で送ってよいスタンプかを返す。
// 送れるのはリアクションと同じ許可リストの、文字スタンプだけ。
func IsAllowedMessageStamp(id string) bool {
	return strings.HasPrefix(id, StampIDPrefix) && IsAllowedMessageReactionEmoji(id)
}

// allowedMessageReactionEmojis は付けてよい絵文字。
//
// 授業内チャットの世界観に合わないもの（Badサイン、中指など）を入れないため、
// 自由入力は受け付けず、ここに挙げたものだけを許す。いまは文字スタンプだけ
// （基本の絵文字は、本人の判断で外した）。増やすときはここに足すだけでよい
// （クライアントは messageReactionEmojis クエリでこの一覧を取るので、画面側の
// 修正は要らない）。
var allowedMessageReactionEmojis = []string{
	// 文字スタンプ。意味がはっきり伝わるように、文字で言い切るもの。
	// "stamp:" で始まるIDで持ち、見た目（文言・色）は画面側が決める
	// （client の constants/reactionStamps.ts）。画像のスタンプを足すときも同じIDで持てる。
	"stamp:thanks",
	"stamp:kansha",
	"stamp:tasukaru",
	"stamp:kami",
	"stamp:naruhodo",
	"stamp:tame",
	"stamp:ryokai",
	"stamp:wakaru",
	"stamp:watashi",
	"stamp:shirabe",
	"stamp:jikan",
	"stamp:makasete",
	"stamp:kaiketsu",
	"stamp:gomen",
}

// AllowedMessageReactionEmojis は付けてよい絵文字の一覧を返す（呼び出し側が
// 書き換えても元の一覧が変わらないようコピーを返す）。
func AllowedMessageReactionEmojis() []string {
	out := make([]string, len(allowedMessageReactionEmojis))
	copy(out, allowedMessageReactionEmojis)
	return out
}

// IsAllowedMessageReactionEmoji は emoji が付けてよい絵文字かを返す。
func IsAllowedMessageReactionEmoji(emoji string) bool {
	for _, e := range allowedMessageReactionEmojis {
		if e == emoji {
			return true
		}
	}
	return false
}
