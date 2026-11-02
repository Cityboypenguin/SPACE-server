package question

import (
	"unicode/utf8"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
	"github.com/Cityboypenguin/SPACE-server/usecase/ngword"
)

const MaxBodyChars = 1000

func validateBody(body string) error {
	if utf8.RuneCountInString(body) > MaxBodyChars {
		return apperr.InvalidInput("質問は1000文字以内で入力してください")
	}
	return nil
}

// Option は質問・回答のユースケースの任意の部品。
type Option func(*options)

type options struct {
	ng ngword.Guard
}

// WithNGWordGuard は本文を保存の前に、NGワードで検査する。権限・存在の確認を済ませた
// あとに呼ぶので、投稿できない人の試行は数えない。
func WithNGWordGuard(g ngword.Guard) Option {
	return func(o *options) { o.ng = g }
}

func buildOptions(opts []Option) options {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
